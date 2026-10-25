//go:build linux || windows

package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Kazzer/zboard-merc/internal/hidrd"
)

func reportIDs(c collection, kind hidrd.Kind) []uint8 {
	if raw := c.Descriptor(); raw != nil {
		if d, err := hidrd.Parse(raw); err == nil {
			return d.ReportIDs(kind)
		}
	}
	return []uint8{0, 1, 2, 3}
}

func staticCapture(colls []collection, out, model string) {
	var list []map[string]any
	for _, c := range colls {
		info := c.Info()
		e := map[string]any{
			"name": info.Name, "product": info.Product,
			"vendor_id": fmt.Sprintf("%04x", info.VendorID), "product_id": fmt.Sprintf("%04x", info.ProductID), "version": fmt.Sprintf("%04x", info.Version),
			"usage_page": fmt.Sprintf("%04X", info.UsagePage), "usage": fmt.Sprintf("%04X", info.Usage),
			"input_len": info.InputLen, "output_len": info.OutputLen, "feature_len": info.FeatureLen, "readable": info.Readable,
			"details": c.Details(),
		}
		reports := map[string]string{}
		for _, id := range reportIDs(c, hidrd.Input) {
			rep, err := c.GetInput(id)
			reports[fmt.Sprintf("input:%d", id)] = hexOrError(rep, err)
		}
		if info.FeatureLen > 0 {
			for _, id := range reportIDs(c, hidrd.Feature) {
				rep, err := c.GetFeature(id)
				reports[fmt.Sprintf("feature:%d", id)] = hexOrError(rep, err)
			}
		}
		e["reports"] = reports
		if desc := c.Descriptor(); desc != nil {
			e["descriptor"] = hex.EncodeToString(desc)
			os.WriteFile(filepath.Join(out, "report-descriptor-"+info.Name+".bin"), desc, 0o644)
		}
		list = append(list, e)
		access := ""
		if !info.Readable {
			access = " (no access: keyboard collection)"
		}
		fmt.Printf("  %-12s %-30s vid %04x pid %04x ver %04x page %04X usage %04X in %d out %d feat %d%s\n", info.Name, info.Product,
			info.VendorID, info.ProductID, info.Version, info.UsagePage, info.Usage, info.InputLen, info.OutputLen, info.FeatureLen, access)
	}
	doc := map[string]any{"os": runtime.GOOS, "model": model, "pid": fmt.Sprintf("%04x", colls[0].Info().ProductID), "collections": list}
	if usb, descriptors := usbInfo(colls); usb != nil {
		doc["usb"] = usb
		if descriptors != nil {
			os.WriteFile(filepath.Join(out, "usb-descriptors.bin"), descriptors, 0o644)
		}
	}
	b, _ := json.MarshalIndent(doc, "", "  ")
	os.WriteFile(filepath.Join(out, "static.json"), b, 0o644)
}

func hexOrError(rep []byte, err error) string {
	if err != nil {
		return err.Error()
	}
	return hex.EncodeToString(rep)
}

func writeTar(dir string) (string, error) {
	path := strings.TrimRight(dir, `\/`) + ".tar.gz"
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	base := filepath.Base(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			return "", err
		}
		h, _ := tar.FileInfoHeader(info, "")
		h.Name = base + "/" + e.Name()
		tw.WriteHeader(h)
		src, err := os.Open(filepath.Join(dir, e.Name()))
		if err != nil {
			return "", err
		}
		io.Copy(tw, src)
		src.Close()
	}
	return path, nil
}
