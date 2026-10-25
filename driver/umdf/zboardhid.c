#include "zboardhid.h"

//  0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E,
static const UCHAR translate[11] = {
    0x87, 0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x92, 0x93, 0x70, 0x73, 0x85,
};

static void
PutNumber(char **At, const char *End, unsigned long Value, unsigned Base)
{
    char digits[16];
    int n = 0;
    do {
        digits[n++] = "0123456789abcdef"[Value % Base];
        Value /= Base;
    } while (Value != 0 && n < (int)sizeof(digits));
    while (n > 0 && *At < End) {
        *(*At)++ = digits[--n];
    }
}

void
Trace(_In_ PCSTR Format, ...)
{
    char line[256];
    char *at = line;
    const char *end = line + sizeof(line) - 1;
    va_list args;

    va_start(args, Format);
    while (*Format != '\0' && at < end) {
        if (*Format != '%') {
            *at++ = *Format++;
            continue;
        }
        Format++;
        switch (*Format != '\0' ? *Format++ : '\0') {
        case 'u':
            PutNumber(&at, end, va_arg(args, unsigned), 10);
            break;
        case 'x':
            PutNumber(&at, end, va_arg(args, unsigned), 16);
            break;
        case '%':
            *at++ = '%';
            break;
        default:
            break;
        }
    }
    va_end(args);
    *at = '\0';
    OutputDebugStringA(line);
}

static NTSTATUS CreateQueues(_In_ WDFDEVICE Device);
static NTSTATUS ReadDescriptors(_In_ PDEVICE_CONTEXT Context);
static NTSTATUS ControlTransfer(_In_ PDEVICE_CONTEXT Context, _In_ UCHAR Direction, _In_ UCHAR Recipient,
                                _In_ BOOLEAN ClassRequest, _In_ UCHAR Request, _In_ USHORT Value,
                                _Inout_opt_ PVOID Buffer, _In_ ULONG Length, _Out_opt_ PULONG Transferred);
static NTSTATUS CopyToRequest(_In_ WDFREQUEST Request, _In_reads_bytes_(Length) PVOID Source, _In_ size_t Length);
static NTSTATUS SetLeds(_In_ PDEVICE_CONTEXT Context, _In_ UCHAR Leds);
static NTSTATUS GetReport(_In_ PDEVICE_CONTEXT Context, _In_ WDFREQUEST Request, _In_ UCHAR ReportType);
static NTSTATUS SetReport(_In_ PDEVICE_CONTEXT Context, _In_ WDFREQUEST Request, _In_ UCHAR ReportType);
static VOID StartAgentChannel(_In_ PDEVICE_CONTEXT Context);
static VOID StopAgentChannel(_In_ PDEVICE_CONTEXT Context);

NTSTATUS
DriverEntry(_In_ PDRIVER_OBJECT DriverObject, _In_ PUNICODE_STRING RegistryPath)
{
    WDF_DRIVER_CONFIG config;

    WDF_DRIVER_CONFIG_INIT(&config, ZbEvtDeviceAdd);
    return WdfDriverCreate(DriverObject, RegistryPath, WDF_NO_OBJECT_ATTRIBUTES, &config, WDF_NO_HANDLE);
}

NTSTATUS
ZbEvtDeviceAdd(_In_ WDFDRIVER Driver, _Inout_ PWDFDEVICE_INIT DeviceInit)
{
    NTSTATUS status;
    WDF_OBJECT_ATTRIBUTES attributes;
    WDF_PNPPOWER_EVENT_CALLBACKS pnp;
    WDF_DEVICE_PNP_CAPABILITIES capabilities;
    WDFDEVICE device;
    PDEVICE_CONTEXT context;

    UNREFERENCED_PARAMETER(Driver);

    WdfFdoInitSetFilter(DeviceInit);

    WDF_PNPPOWER_EVENT_CALLBACKS_INIT(&pnp);
    pnp.EvtDevicePrepareHardware = ZbEvtDevicePrepareHardware;
    pnp.EvtDeviceD0Entry = ZbEvtDeviceD0Entry;
    pnp.EvtDeviceD0Exit = ZbEvtDeviceD0Exit;
    pnp.EvtDeviceReleaseHardware = ZbEvtDeviceReleaseHardware;
    WdfDeviceInitSetPnpPowerEventCallbacks(DeviceInit, &pnp);

    WDF_OBJECT_ATTRIBUTES_INIT_CONTEXT_TYPE(&attributes, DEVICE_CONTEXT);
    status = WdfDeviceCreate(&DeviceInit, &attributes, &device);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    context = GetDeviceContext(device);
    RtlZeroMemory(context, sizeof(*context));
    context->Device = device;

    WDF_DEVICE_PNP_CAPABILITIES_INIT(&capabilities);
    capabilities.SurpriseRemovalOK = WdfTrue;
    WdfDeviceSetPnpCapabilities(device, &capabilities);

    WDF_OBJECT_ATTRIBUTES_INIT(&attributes);
    attributes.ParentObject = device;
    status = WdfSpinLockCreate(&attributes, &context->ReportLock);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    return CreateQueues(device);
}

static NTSTATUS
CreateQueues(_In_ WDFDEVICE Device)
{
    NTSTATUS status;
    WDF_IO_QUEUE_CONFIG config;
    PDEVICE_CONTEXT context = GetDeviceContext(Device);

    WDF_IO_QUEUE_CONFIG_INIT_DEFAULT_QUEUE(&config, WdfIoQueueDispatchParallel);
    config.EvtIoDeviceControl = ZbEvtIoDeviceControl;
    status = WdfIoQueueCreate(Device, &config, WDF_NO_OBJECT_ATTRIBUTES, WDF_NO_HANDLE);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    WDF_IO_QUEUE_CONFIG_INIT(&config, WdfIoQueueDispatchManual);
    return WdfIoQueueCreate(Device, &config, WDF_NO_OBJECT_ATTRIBUTES, &context->ReadQueue);
}

NTSTATUS
ZbEvtDevicePrepareHardware(_In_ WDFDEVICE Device, _In_ WDFCMRESLIST Resources, _In_ WDFCMRESLIST ResourcesTranslated)
{
    NTSTATUS status;
    PDEVICE_CONTEXT context = GetDeviceContext(Device);
    WDF_USB_DEVICE_CREATE_CONFIG createConfig;
    WDF_USB_DEVICE_SELECT_CONFIG_PARAMS configParams;
    WDF_USB_CONTINUOUS_READER_CONFIG readerConfig;
    USB_DEVICE_DESCRIPTOR deviceDescriptor;
    WDF_USB_PIPE_INFORMATION pipeInfo;
    UCHAR pipes, i;

    UNREFERENCED_PARAMETER(Resources);
    UNREFERENCED_PARAMETER(ResourcesTranslated);

    if (context->UsbDevice == NULL) {
        WDF_USB_DEVICE_CREATE_CONFIG_INIT(&createConfig, USBD_CLIENT_CONTRACT_VERSION_602);
        status = WdfUsbTargetDeviceCreateWithParameters(Device, &createConfig, WDF_NO_OBJECT_ATTRIBUTES, &context->UsbDevice);
        if (!NT_SUCCESS(status)) {
            return status;
        }
    }

    WdfUsbTargetDeviceGetDeviceDescriptor(context->UsbDevice, &deviceDescriptor);
    context->Attributes.Size = sizeof(HID_DEVICE_ATTRIBUTES);
    context->Attributes.VendorID = deviceDescriptor.idVendor;
    context->Attributes.ProductID = deviceDescriptor.idProduct;
    context->Attributes.VersionNumber = deviceDescriptor.bcdDevice;

    WDF_USB_DEVICE_SELECT_CONFIG_PARAMS_INIT_SINGLE_INTERFACE(&configParams);
    status = WdfUsbTargetDeviceSelectConfig(context->UsbDevice, WDF_NO_OBJECT_ATTRIBUTES, &configParams);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    context->UsbInterface = configParams.Types.SingleInterface.ConfiguredUsbInterface;
    context->InterfaceNumber = WdfUsbInterfaceGetInterfaceNumber(context->UsbInterface);
    pipes = configParams.Types.SingleInterface.NumberConfiguredPipes;
    context->InterruptPipe = NULL;
    for (i = 0; i < pipes; i++) {
        WDFUSBPIPE pipe;
        WDF_USB_PIPE_INFORMATION_INIT(&pipeInfo);
        pipe = WdfUsbInterfaceGetConfiguredPipe(context->UsbInterface, i, &pipeInfo);
        if (pipeInfo.PipeType == WdfUsbPipeTypeInterrupt && WdfUsbTargetPipeIsInEndpoint(pipe)) {
            WdfUsbTargetPipeSetNoMaximumPacketSizeCheck(pipe);
            context->InterruptPipe = pipe;
            context->InputReportLength = pipeInfo.MaximumPacketSize;
            break;
        }
    }
    if (context->InterruptPipe == NULL || context->InputReportLength == 0 || context->InputReportLength > MAX_INPUT_REPORT) {
        Trace("zboardhid: no interrupt-in pipe (pipes %u, report length %u)\n", (unsigned)pipes, (unsigned)context->InputReportLength);
        return STATUS_DEVICE_CONFIGURATION_ERROR;
    }

    status = ReadDescriptors(context);
    if (!NT_SUCCESS(status)) {
        Trace("zboardhid: descriptors failed 0x%x\n", (unsigned)status);
        return status;
    }
    Trace("zboardhid: interface %u, %x:%x, report descriptor %u bytes, input report %u bytes\n",
          (unsigned)context->InterfaceNumber, (unsigned)context->Attributes.VendorID, (unsigned)context->Attributes.ProductID,
          (unsigned)context->ReportDescriptorLength, (unsigned)context->InputReportLength);

    status = ControlTransfer(context, BmRequestHostToDevice, BmRequestToInterface, TRUE, USB_REQUEST_SET_IDLE, 0, NULL, 0, NULL);
    if (!NT_SUCCESS(status)) {
        Trace("zboardhid: set idle failed 0x%x\n", (unsigned)status);
    }
    status = ControlTransfer(context, BmRequestHostToDevice, BmRequestToInterface, TRUE, USB_REQUEST_SET_PROTOCOL, 1, NULL, 0, NULL);
    if (!NT_SUCCESS(status)) {
        Trace("zboardhid: set protocol failed 0x%x\n", (unsigned)status);
    }

    SetLeds(context, LED_NUM);

    WDF_USB_CONTINUOUS_READER_CONFIG_INIT(&readerConfig, ZbEvtInterruptRead, context, context->InputReportLength);
    readerConfig.EvtUsbTargetPipeReadersFailed = ZbEvtInterruptReadersFailed;
    status = WdfUsbTargetPipeConfigContinuousReader(context->InterruptPipe, &readerConfig);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    StartAgentChannel(context);
    return STATUS_SUCCESS;
}

static NTSTATUS
ReadDescriptors(_In_ PDEVICE_CONTEXT Context)
{
    NTSTATUS status;
    ULONG got = 0;
    USHORT length;

    status = ControlTransfer(Context, BmRequestDeviceToHost, BmRequestToInterface, FALSE, USB_REQUEST_GET_DESCRIPTOR,
                             USB_HID_DESCRIPTOR_TYPE << 8, &Context->HidDescriptor, sizeof(HID_DESCRIPTOR), &got);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    if (got < sizeof(HID_DESCRIPTOR) || Context->HidDescriptor.bNumDescriptors < 1) {
        return STATUS_DEVICE_DATA_ERROR;
    }
    length = Context->HidDescriptor.DescriptorList[0].wReportLength;
    if (length == 0 || length > MAX_REPORT_DESCRIPTOR) {
        return STATUS_DEVICE_DATA_ERROR;
    }
    status = ControlTransfer(Context, BmRequestDeviceToHost, BmRequestToInterface, FALSE, USB_REQUEST_GET_DESCRIPTOR,
                             USB_REPORT_DESCRIPTOR_TYPE << 8, Context->ReportDescriptor, length, &got);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    if (got != length) {
        return STATUS_DEVICE_DATA_ERROR;
    }
    Context->ReportDescriptorLength = length;
    return STATUS_SUCCESS;
}

static NTSTATUS
ControlTransfer(_In_ PDEVICE_CONTEXT Context, _In_ UCHAR Direction, _In_ UCHAR Recipient, _In_ BOOLEAN ClassRequest,
                _In_ UCHAR Request, _In_ USHORT Value, _Inout_opt_ PVOID Buffer, _In_ ULONG Length, _Out_opt_ PULONG Transferred)
{
    WDF_USB_CONTROL_SETUP_PACKET setup;
    WDF_MEMORY_DESCRIPTOR memory;
    WDF_REQUEST_SEND_OPTIONS options;

    if (ClassRequest) {
        WDF_USB_CONTROL_SETUP_PACKET_INIT_CLASS(&setup, Direction, Recipient, Request, Value, Context->InterfaceNumber);
    } else {
        WDF_USB_CONTROL_SETUP_PACKET_INIT(&setup, Direction, Recipient, Request, Value, Context->InterfaceNumber);
    }
    WDF_REQUEST_SEND_OPTIONS_INIT(&options, 0);
    WDF_REQUEST_SEND_OPTIONS_SET_TIMEOUT(&options, WDF_REL_TIMEOUT_IN_SEC(2));
    if (Buffer != NULL && Length > 0) {
        WDF_MEMORY_DESCRIPTOR_INIT_BUFFER(&memory, Buffer, Length);
        return WdfUsbTargetDeviceSendControlTransferSynchronously(Context->UsbDevice, WDF_NO_HANDLE, &options, &setup, &memory, Transferred);
    }
    return WdfUsbTargetDeviceSendControlTransferSynchronously(Context->UsbDevice, WDF_NO_HANDLE, &options, &setup, NULL, Transferred);
}

NTSTATUS
ZbEvtDeviceD0Entry(_In_ WDFDEVICE Device, _In_ WDF_POWER_DEVICE_STATE PreviousState)
{
    PDEVICE_CONTEXT context = GetDeviceContext(Device);
    UNREFERENCED_PARAMETER(PreviousState);
    return WdfIoTargetStart(WdfUsbTargetPipeGetIoTarget(context->InterruptPipe));
}

NTSTATUS
ZbEvtDeviceD0Exit(_In_ WDFDEVICE Device, _In_ WDF_POWER_DEVICE_STATE TargetState)
{
    PDEVICE_CONTEXT context = GetDeviceContext(Device);
    UNREFERENCED_PARAMETER(TargetState);
    WdfIoTargetStop(WdfUsbTargetPipeGetIoTarget(context->InterruptPipe), WdfIoTargetCancelSentIo);
    return STATUS_SUCCESS;
}

static DWORD WINAPI
AgentChannelThread(_In_ LPVOID Parameter)
{
    PDEVICE_CONTEXT context = (PDEVICE_CONTEXT)Parameter;
    SECURITY_DESCRIPTOR descriptor;
    SECURITY_ATTRIBUTES attributes;
    DWORD sid[SECURITY_MAX_SID_SIZE / sizeof(DWORD)];
    DWORD sidSize = sizeof(sid);
    DWORD acl[(sizeof(ACL) + sizeof(ACCESS_ALLOWED_ACE) + SECURITY_MAX_SID_SIZE) / sizeof(DWORD)];
    HANDLE pipe = INVALID_HANDLE_VALUE;
    HANDLE io;
    HANDLE waits[2];
    OVERLAPPED overlapped;
    DWORD got, error;
    UCHAR mask;

    io = CreateEventW(NULL, TRUE, FALSE, NULL);
    if (io == NULL) {
        Trace("zboardhid: agent channel: no event, %u\n", (unsigned)GetLastError());
        return 0;
    }
    waits[0] = context->ChannelStop;
    waits[1] = io;
    if (!CreateWellKnownSid(WinInteractiveSid, NULL, (PSID)sid, &sidSize) ||
        !InitializeAcl((PACL)acl, sizeof(acl), ACL_REVISION) ||
        !AddAccessAllowedAce((PACL)acl, ACL_REVISION, FILE_GENERIC_WRITE, (PSID)sid) ||
        !InitializeSecurityDescriptor(&descriptor, SECURITY_DESCRIPTOR_REVISION) ||
        !SetSecurityDescriptorDacl(&descriptor, TRUE, (PACL)acl, FALSE)) {
        Trace("zboardhid: agent channel: no security descriptor, %u\n", (unsigned)GetLastError());
        CloseHandle(io);
        return 0;
    }
    attributes.nLength = sizeof(attributes);
    attributes.lpSecurityDescriptor = &descriptor;
    attributes.bInheritHandle = FALSE;

    for (;;) {
        if (pipe == INVALID_HANDLE_VALUE) {
            pipe = CreateNamedPipeW(ZBOARD_AGENT_PIPE, PIPE_ACCESS_INBOUND | FILE_FLAG_OVERLAPPED | FILE_FLAG_FIRST_PIPE_INSTANCE,
                                    PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS, 1, 16, 16, 0, &attributes);
            if (pipe == INVALID_HANDLE_VALUE) {
                Trace("zboardhid: agent channel: pipe not created, %u\n", (unsigned)GetLastError());
                if (WaitForSingleObject(context->ChannelStop, 30000) == WAIT_OBJECT_0) {
                    break;
                }
                continue;
            }
        }
        RtlZeroMemory(&overlapped, sizeof(overlapped));
        overlapped.hEvent = io;
        ResetEvent(io);
        if (!ConnectNamedPipe(pipe, &overlapped)) {
            error = GetLastError();
            if (error == ERROR_PIPE_CONNECTED) {
                SetEvent(io);
            } else if (error != ERROR_IO_PENDING) {
                Trace("zboardhid: agent channel: connect failed, %u\n", (unsigned)error);
                CloseHandle(pipe);
                pipe = INVALID_HANDLE_VALUE;
                if (WaitForSingleObject(context->ChannelStop, 5000) == WAIT_OBJECT_0) {
                    break;
                }
                continue;
            }
        }
        if (WaitForMultipleObjects(2, waits, FALSE, INFINITE) == WAIT_OBJECT_0) {
            CancelIoEx(pipe, &overlapped);
            GetOverlappedResult(pipe, &overlapped, &got, TRUE);
            break;
        }
        InterlockedExchange(&context->AgentConnected, 1);
        Trace("zboardhid: agent connected\n");
        for (;;) {
            RtlZeroMemory(&overlapped, sizeof(overlapped));
            overlapped.hEvent = io;
            ResetEvent(io);
            if (!ReadFile(pipe, &mask, 1, NULL, &overlapped) && GetLastError() != ERROR_IO_PENDING) {
                break;
            }
            if (WaitForMultipleObjects(2, waits, FALSE, INFINITE) == WAIT_OBJECT_0) {
                CancelIoEx(pipe, &overlapped);
                GetOverlappedResult(pipe, &overlapped, &got, TRUE);
                goto stop;
            }
            if (!GetOverlappedResult(pipe, &overlapped, &got, FALSE) || got != 1) {
                break;
            }
            SetLeds(context, mask);
        }
        InterlockedExchange(&context->AgentConnected, 0);
        Trace("zboardhid: agent disconnected\n");
        DisconnectNamedPipe(pipe);
    }
stop:
    InterlockedExchange(&context->AgentConnected, 0);
    if (pipe != INVALID_HANDLE_VALUE) {
        CloseHandle(pipe);
    }
    CloseHandle(io);
    return 0;
}

static VOID
StartAgentChannel(_In_ PDEVICE_CONTEXT Context)
{
    if (Context->ChannelThread != NULL) {
        return;
    }
    Context->ChannelStop = CreateEventW(NULL, TRUE, FALSE, NULL);
    if (Context->ChannelStop == NULL) {
        Trace("zboardhid: agent channel: no stop event, %u\n", (unsigned)GetLastError());
        return;
    }
    Context->ChannelThread = CreateThread(NULL, 0, AgentChannelThread, Context, 0, NULL);
    if (Context->ChannelThread == NULL) {
        Trace("zboardhid: agent channel: no thread, %u\n", (unsigned)GetLastError());
        CloseHandle(Context->ChannelStop);
        Context->ChannelStop = NULL;
    }
}

static VOID
StopAgentChannel(_In_ PDEVICE_CONTEXT Context)
{
    if (Context->ChannelThread == NULL) {
        return;
    }
    SetEvent(Context->ChannelStop);
    WaitForSingleObject(Context->ChannelThread, INFINITE);
    CloseHandle(Context->ChannelThread);
    CloseHandle(Context->ChannelStop);
    Context->ChannelThread = NULL;
    Context->ChannelStop = NULL;
}

NTSTATUS
ZbEvtDeviceReleaseHardware(_In_ WDFDEVICE Device, _In_ WDFCMRESLIST ResourcesTranslated)
{
    UNREFERENCED_PARAMETER(ResourcesTranslated);
    StopAgentChannel(GetDeviceContext(Device));
    return STATUS_SUCCESS;
}

static UCHAR
LockBit(_In_ UCHAR Usage)
{
    switch (Usage) {
    case USAGE_NUM_LOCK:
        return LED_NUM;
    case USAGE_CAPS_LOCK:
        return LED_CAPS;
    case USAGE_SCROLL_LOCK:
        return LED_SCROLL;
    }
    return 0;
}

static BOOLEAN
Held(_In_reads_bytes_(Length) const UCHAR *Report, _In_ ULONG Length, _In_ UCHAR Usage)
{
    ULONG i;
    for (i = 2; i < Length; i++) {
        if (Report[i] == Usage) {
            return TRUE;
        }
    }
    return FALSE;
}

static NTSTATUS
SetLeds(_In_ PDEVICE_CONTEXT Context, _In_ UCHAR Leds)
{
    NTSTATUS status;
    UCHAR report = Leds & (LED_NUM | LED_CAPS | LED_SCROLL);

    status = ControlTransfer(Context, BmRequestHostToDevice, BmRequestToInterface, TRUE, USB_REQUEST_SET_REPORT,
                             HID_REPORT_TYPE_OUTPUT << 8, &report, 1, NULL);
    if (NT_SUCCESS(status)) {
        WdfSpinLockAcquire(Context->ReportLock);
        Context->Leds = report;
        WdfSpinLockRelease(Context->ReportLock);
    }
    Trace("zboardhid: leds %x: 0x%x\n", (unsigned)report, (unsigned)status);
    return status;
}

VOID
ZbEvtInterruptRead(_In_ WDFUSBPIPE Pipe, _In_ WDFMEMORY Buffer, _In_ size_t NumBytesTransferred, _In_ WDFCONTEXT Context)
{
    PDEVICE_CONTEXT context = (PDEVICE_CONTEXT)Context;
    PUCHAR report;
    UCHAR copy[MAX_INPUT_REPORT];
    UCHAR toggled, leds = 0;
    size_t i;
    WDFREQUEST request;
    NTSTATUS status;

    UNREFERENCED_PARAMETER(Pipe);
    if (NumBytesTransferred == 0 || NumBytesTransferred > MAX_INPUT_REPORT) {
        return;
    }
    report = (PUCHAR)WdfMemoryGetBuffer(Buffer, NULL);
    RtlCopyMemory(copy, report, NumBytesTransferred);

    for (i = 2; i < NumBytesTransferred; i++) {
        if (copy[i] >= 0x74 && copy[i] <= 0x7E) {
            copy[i] = translate[copy[i] - 0x74];
        }
    }

    toggled = 0;
    WdfSpinLockAcquire(context->ReportLock);
    for (i = 2; i < NumBytesTransferred; i++) {
        UCHAR bit = LockBit(copy[i]);
        if (bit != 0 && !Held(context->LastReport, context->LastReportLength, copy[i])) {
            toggled |= bit;
        }
    }
    RtlCopyMemory(context->LastReport, copy, NumBytesTransferred);
    context->LastReportLength = (ULONG)NumBytesTransferred;
    if (toggled != 0 && context->AgentConnected == 0) {
        context->Leds ^= toggled;
        leds = context->Leds;
    } else {
        toggled = 0;
    }
    WdfSpinLockRelease(context->ReportLock);
    if (toggled != 0) {
        SetLeds(context, leds);
    }

    status = WdfIoQueueRetrieveNextRequest(context->ReadQueue, &request);
    if (!NT_SUCCESS(status)) {
        return;
    }
    status = CopyToRequest(request, copy, NumBytesTransferred);
    WdfRequestComplete(request, status);
}

BOOLEAN
ZbEvtInterruptReadersFailed(_In_ WDFUSBPIPE Pipe, _In_ NTSTATUS Status, _In_ USBD_STATUS UsbdStatus)
{
    UNREFERENCED_PARAMETER(Pipe);
    UNREFERENCED_PARAMETER(Status);
    UNREFERENCED_PARAMETER(UsbdStatus);
    return TRUE;
}

static NTSTATUS
CopyToRequest(_In_ WDFREQUEST Request, _In_reads_bytes_(Length) PVOID Source, _In_ size_t Length)
{
    NTSTATUS status;
    WDFMEMORY memory;
    size_t available;

    status = WdfRequestRetrieveOutputMemory(Request, &memory);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    WdfMemoryGetBuffer(memory, &available);
    if (available < Length) {
        return STATUS_INVALID_BUFFER_SIZE;
    }
    status = WdfMemoryCopyFromBuffer(memory, 0, Source, Length);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    WdfRequestSetInformation(Request, Length);
    return STATUS_SUCCESS;
}

static NTSTATUS
GetReport(_In_ PDEVICE_CONTEXT Context, _In_ WDFREQUEST Request, _In_ UCHAR ReportType)
{
    NTSTATUS status;
    WDFMEMORY inMemory, outMemory;
    PUCHAR in, out;
    size_t inLength, outLength;
    UCHAR reportId;
    ULONG got = 0;

    status = WdfRequestRetrieveInputMemory(Request, &inMemory);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    in = (PUCHAR)WdfMemoryGetBuffer(inMemory, &inLength);
    if (inLength < 1) {
        return STATUS_INVALID_BUFFER_SIZE;
    }
    reportId = in[0];
    status = WdfRequestRetrieveOutputMemory(Request, &outMemory);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    out = (PUCHAR)WdfMemoryGetBuffer(outMemory, &outLength);
    if (outLength < 1) {
        return STATUS_INVALID_BUFFER_SIZE;
    }
    if (ReportType == HID_REPORT_TYPE_INPUT && reportId == 0) {
        WdfSpinLockAcquire(Context->ReportLock);
        got = Context->LastReportLength;
        if (got > outLength) {
            got = (ULONG)outLength;
        }
        RtlCopyMemory(out, Context->LastReport, got);
        WdfSpinLockRelease(Context->ReportLock);
    } else {
        status = ControlTransfer(Context, BmRequestDeviceToHost, BmRequestToInterface, TRUE, USB_REQUEST_GET_REPORT,
                                 (USHORT)((ReportType << 8) | reportId), out, (ULONG)outLength, &got);
        if (!NT_SUCCESS(status)) {
            return status;
        }
    }
    WdfRequestSetInformation(Request, got);
    return STATUS_SUCCESS;
}

static NTSTATUS
SetReport(_In_ PDEVICE_CONTEXT Context, _In_ WDFREQUEST Request, _In_ UCHAR ReportType)
{
    NTSTATUS status;
    WDFMEMORY inMemory, outMemory;
    PUCHAR in;
    size_t inLength, outLength = 0;
    UCHAR reportId;

    status = WdfRequestRetrieveOutputMemory(Request, &outMemory);
    if (NT_SUCCESS(status)) {
        WdfMemoryGetBuffer(outMemory, &outLength);
    }
    reportId = (UCHAR)outLength;
    status = WdfRequestRetrieveInputMemory(Request, &inMemory);
    if (!NT_SUCCESS(status)) {
        Trace("zboardhid: set report type %u id %u: no input buffer, 0x%x\n", (unsigned)ReportType, (unsigned)reportId, (unsigned)status);
        return status;
    }
    in = (PUCHAR)WdfMemoryGetBuffer(inMemory, &inLength);
    if (inLength < 1) {
        Trace("zboardhid: set report type %u id %u: empty buffer\n", (unsigned)ReportType, (unsigned)reportId);
        return STATUS_INVALID_BUFFER_SIZE;
    }
    status = ControlTransfer(Context, BmRequestHostToDevice, BmRequestToInterface, TRUE, USB_REQUEST_SET_REPORT,
                             (USHORT)((ReportType << 8) | reportId), in, (ULONG)inLength, NULL);
    if (!NT_SUCCESS(status)) {
        return status;
    }
    WdfRequestSetInformation(Request, inLength);
    return STATUS_SUCCESS;
}

VOID
ZbEvtIoDeviceControl(_In_ WDFQUEUE Queue, _In_ WDFREQUEST Request, _In_ size_t OutputBufferLength,
                     _In_ size_t InputBufferLength, _In_ ULONG IoControlCode)
{
    PDEVICE_CONTEXT context = GetDeviceContext(WdfIoQueueGetDevice(Queue));
    NTSTATUS status;

    UNREFERENCED_PARAMETER(OutputBufferLength);
    UNREFERENCED_PARAMETER(InputBufferLength);

    switch (IoControlCode) {
    case IOCTL_HID_GET_DEVICE_DESCRIPTOR:
        status = CopyToRequest(Request, &context->HidDescriptor, sizeof(HID_DESCRIPTOR));
        break;
    case IOCTL_HID_GET_DEVICE_ATTRIBUTES:
        status = CopyToRequest(Request, &context->Attributes, sizeof(HID_DEVICE_ATTRIBUTES));
        break;
    case IOCTL_HID_GET_REPORT_DESCRIPTOR:
        status = CopyToRequest(Request, context->ReportDescriptor, context->ReportDescriptorLength);
        break;
    case IOCTL_HID_READ_REPORT:
        status = WdfRequestForwardToIoQueue(Request, context->ReadQueue);
        if (NT_SUCCESS(status)) {
            return;
        }
        break;
    case IOCTL_HID_WRITE_REPORT:
    case IOCTL_HID_SET_OUTPUT_REPORT:
    case IOCTL_UMDF_HID_SET_OUTPUT_REPORT:
        status = SetReport(context, Request, HID_REPORT_TYPE_OUTPUT);
        break;
    case IOCTL_HID_SET_FEATURE:
    case IOCTL_UMDF_HID_SET_FEATURE:
        status = SetReport(context, Request, HID_REPORT_TYPE_FEATURE);
        break;
    case IOCTL_HID_GET_FEATURE:
    case IOCTL_UMDF_HID_GET_FEATURE:
        status = GetReport(context, Request, HID_REPORT_TYPE_FEATURE);
        break;
    case IOCTL_HID_GET_INPUT_REPORT:
    case IOCTL_UMDF_HID_GET_INPUT_REPORT:
        status = GetReport(context, Request, HID_REPORT_TYPE_INPUT);
        break;
    case IOCTL_HID_ACTIVATE_DEVICE:
    case IOCTL_HID_DEACTIVATE_DEVICE:
        status = STATUS_SUCCESS;
        break;
    default:
        status = STATUS_NOT_SUPPORTED;
        break;
    }
    WdfRequestComplete(Request, status);
}
