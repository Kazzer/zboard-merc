#pragma once

#define WIN32_NO_STATUS
#include <windows.h>
#undef WIN32_NO_STATUS
#include <winioctl.h>
#pragma warning(disable : 4201)
#include <ntstatus.h>
#include <devpropdef.h>
#include <wudfwdm.h>
typedef struct _IO_STACK_LOCATION *PIO_STACK_LOCATION;
#include <usbdi.h>
#include <wdf.h>
#include <wdfusb.h>
#include <hidport.h>
#include <stdarg.h>

#ifndef USBD_CLIENT_CONTRACT_VERSION_602
#define USBD_CLIENT_CONTRACT_VERSION_602 0x602
#endif

#define ZBOARD_AGENT_PIPE L"\\\\.\\pipe\\zboardhid"
#define LED_NUM 0x01
#define LED_CAPS 0x02
#define LED_SCROLL 0x04
#define USAGE_CAPS_LOCK 0x39
#define USAGE_SCROLL_LOCK 0x47
#define USAGE_NUM_LOCK 0x53

void Trace(_In_ PCSTR Format, ...);

#define USB_HID_DESCRIPTOR_TYPE 0x21
#define USB_REPORT_DESCRIPTOR_TYPE 0x22
#define USB_REQUEST_GET_REPORT 0x01
#define USB_REQUEST_SET_REPORT 0x09
#define USB_REQUEST_SET_IDLE 0x0A
#define USB_REQUEST_SET_PROTOCOL 0x0B
#define HID_REPORT_TYPE_INPUT 1
#define HID_REPORT_TYPE_OUTPUT 2
#define HID_REPORT_TYPE_FEATURE 3

#define MAX_REPORT_DESCRIPTOR 512
#define MAX_INPUT_REPORT 64

typedef struct _DEVICE_CONTEXT {
    WDFDEVICE Device;
    WDFUSBDEVICE UsbDevice;
    WDFUSBINTERFACE UsbInterface;
    WDFUSBPIPE InterruptPipe;
    UCHAR InterfaceNumber;
    WDFQUEUE ReadQueue;
    HID_DEVICE_ATTRIBUTES Attributes;
    HID_DESCRIPTOR HidDescriptor;
    UCHAR ReportDescriptor[MAX_REPORT_DESCRIPTOR];
    USHORT ReportDescriptorLength;
    ULONG InputReportLength;
    UCHAR LastReport[MAX_INPUT_REPORT];
    ULONG LastReportLength;
    UCHAR Leds;
    WDFSPINLOCK ReportLock;
    HANDLE ChannelThread;
    HANDLE ChannelStop;
    volatile LONG AgentConnected;
} DEVICE_CONTEXT, *PDEVICE_CONTEXT;

WDF_DECLARE_CONTEXT_TYPE_WITH_NAME(DEVICE_CONTEXT, GetDeviceContext);

DRIVER_INITIALIZE DriverEntry;
EVT_WDF_DRIVER_DEVICE_ADD ZbEvtDeviceAdd;
EVT_WDF_DEVICE_PREPARE_HARDWARE ZbEvtDevicePrepareHardware;
EVT_WDF_DEVICE_D0_ENTRY ZbEvtDeviceD0Entry;
EVT_WDF_DEVICE_D0_EXIT ZbEvtDeviceD0Exit;
EVT_WDF_DEVICE_RELEASE_HARDWARE ZbEvtDeviceReleaseHardware;
EVT_WDF_IO_QUEUE_IO_DEVICE_CONTROL ZbEvtIoDeviceControl;
EVT_WDF_USB_READER_COMPLETION_ROUTINE ZbEvtInterruptRead;
EVT_WDF_USB_READERS_FAILED ZbEvtInterruptReadersFailed;
