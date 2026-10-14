package detect

import (
	"bytes"
	"encoding/binary"
	"io"
	"os"
	"strings"

	"github.com/FuadTesfaye/layerpilot/internal/domain"
)

const (
	rpmTagHeaderSignatures = 62
	rpmTagHeaderImmutable  = 63
	rpmTagName             = 1000
	rpmTagVersion          = 1001
	rpmTagRelease          = 1002
	rpmTagSummary          = 1004
	rpmTagDescription      = 1005
	rpmTagGroup            = 1016
	rpmTagArch             = 1022
)

func (d *Detector) inspectRPM(f *os.File, info *domain.PackageInfo) (*domain.PackageInfo, error) {
	info.Format = domain.FormatRPM

	// Lead is 96 bytes
	lead := make([]byte, 96)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot seek to beginning of RPM", err)
	}
	if _, err := io.ReadFull(f, lead); err != nil {
		return nil, domain.WrapError(domain.ErrCodePackageInvalid, "cannot read RPM lead", err)
	}

	// Name in lead (null-terminated string at offset 10..76)
	leadName := string(bytes.TrimRight(lead[10:76], "\x00"))
	if leadName != "" {
		info.Name = leadName
	}

	// Read Signature Header
	sigDataSize, sigEntries, err := readRPMHeaderSection(f)
	if err != nil {
		// If signature parsing fails, use lead name and return
		return info, nil
	}
	// Skip signature index records (16 bytes each) + data store
	sigTotalBytes := int64(sigEntries)*16 + int64(sigDataSize)
	// Align to 8 bytes
	if sigTotalBytes%8 != 0 {
		sigTotalBytes += 8 - (sigTotalBytes % 8)
	}
	if _, err := f.Seek(sigTotalBytes, io.SeekCurrent); err != nil {
		return info, nil
	}

	// Read Main RPM Header
	mainDataSize, mainEntries, err := readRPMHeaderSection(f)
	if err != nil {
		return info, nil
	}

	// Read index entries
	indexRecords := make([]byte, mainEntries*16)
	if _, err := io.ReadFull(f, indexRecords); err != nil {
		return info, nil
	}

	// Read data store
	dataStore := make([]byte, mainDataSize)
	if _, err := io.ReadFull(f, dataStore); err != nil {
		return info, nil
	}

	// Parse main header tags
	for i := 0; i < mainEntries; i++ {
		offset := i * 16
		tag := binary.BigEndian.Uint32(indexRecords[offset : offset+4])
		tagType := binary.BigEndian.Uint32(indexRecords[offset+4 : offset+8])
		dataOffset := binary.BigEndian.Uint32(indexRecords[offset+8 : offset+12])

		if int(dataOffset) >= len(dataStore) {
			continue
		}

		val := extractRPMString(dataStore[dataOffset:], tagType)
		switch tag {
		case rpmTagName:
			if val != "" {
				info.Name = val
			}
		case rpmTagVersion:
			info.Version = val
		case rpmTagRelease:
			if info.Version != "" && val != "" {
				info.Version = info.Version + "-" + val
			}
		case rpmTagArch:
			info.Architecture = normalizeArch(val)
		case rpmTagSummary:
			info.Description = val
		case rpmTagGroup:
			groupLower := strings.ToLower(val)
			if strings.Contains(groupLower, "gui") ||
				strings.Contains(groupLower, "desktop") ||
				strings.Contains(groupLower, "graphics") ||
				strings.Contains(groupLower, "x11") {
				info.PackageType = domain.PackageTypeGUI
			}
		}
	}

	return info, nil
}

func readRPMHeaderSection(f *os.File) (dataSize int, entries int, err error) {
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return 0, 0, err
	}
	// Check magic: \x8e\xad\xe8\x01
	if hdr[0] != 0x8e || hdr[1] != 0xad || hdr[2] != 0xe8 || hdr[3] != 0x01 {
		return 0, 0, domain.NewError(domain.ErrCodePackageInvalid, "invalid RPM header magic", "Verify RPM file integrity", "Inspection failed")
	}

	entries = int(binary.BigEndian.Uint32(hdr[8:12]))
	dataSize = int(binary.BigEndian.Uint32(hdr[12:16]))
	return dataSize, entries, nil
}

func extractRPMString(data []byte, tagType uint32) string {
	nullIdx := bytes.IndexByte(data, 0)
	if nullIdx == -1 {
		return string(data)
	}
	return string(data[:nullIdx])
}
