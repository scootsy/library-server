package scanner

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// mp4Parser performs a minimal parse of an MP4/M4B file to extract iTunes
// metadata tags, duration, chapter data, and embedded cover art.
//
// Only the atoms needed for Phase 1 are decoded. This is intentionally
// lightweight — a full MP4 demuxer would be used in a later phase.
type mp4Parser struct {
	r io.ReadSeeker

	// iTunes metadata (ilst atoms)
	itunesTitle       string
	itunesAlbum       string
	itunesAlbumArtist string
	itunesArtists     []string
	itunesYear        string
	itunesDescription string
	itunesPublisher   string

	// Audiobook
	durationSeconds int
	chapters        []AudioChapter

	// Cover art
	coverData []byte
	coverExt  string
}

func (p *mp4Parser) parse() error {
	// Walk the top-level atoms looking for 'moov'.
	return p.walkAtoms(0, -1, p.handleTopLevel)
}

func (p *mp4Parser) handleTopLevel(name string, data []byte) error {
	switch name {
	case "moov":
		return p.parseMoov(data)
	}
	return nil
}

func (p *mp4Parser) parseMoov(data []byte) error {
	return walkAtomBytes(data, func(name string, payload []byte) error {
		switch name {
		case "mvhd":
			p.parseMvhd(payload)
		case "udta":
			return p.parseUdta(payload)
		case "trak":
			return p.parseTrak(payload)
		}
		return nil
	})
}

// parseMvhd extracts the movie header (duration, timescale).
func (p *mp4Parser) parseMvhd(data []byte) {
	if len(data) < 20 {
		return
	}
	version := data[0]
	if version == 1 {
		if len(data) < 36 {
			return
		}
		timescale := binary.BigEndian.Uint32(data[20:24])
		duration := binary.BigEndian.Uint64(data[24:32])
		if timescale > 0 {
			p.durationSeconds = int(duration / uint64(timescale))
		}
	} else {
		timescale := binary.BigEndian.Uint32(data[12:16])
		duration := binary.BigEndian.Uint32(data[16:20])
		if timescale > 0 {
			p.durationSeconds = int(uint64(duration) / uint64(timescale))
		}
	}
}

func (p *mp4Parser) parseUdta(data []byte) error {
	return walkAtomBytes(data, func(name string, payload []byte) error {
		if name == "meta" {
			return p.parseMeta(payload)
		}
		if name == "chpl" {
			p.parseChpl(payload)
		}
		return nil
	})
}

func (p *mp4Parser) parseMeta(data []byte) error {
	// meta box starts with a 4-byte version/flags field.
	if len(data) < 4 {
		return nil
	}
	return walkAtomBytes(data[4:], func(name string, payload []byte) error {
		if name == "ilst" {
			return p.parseIlst(payload)
		}
		return nil
	})
}

func (p *mp4Parser) parseIlst(data []byte) error {
	return walkAtomBytes(data, func(name string, payload []byte) error {
		return p.handleIlstItem(name, payload)
	})
}

func (p *mp4Parser) handleIlstItem(name string, data []byte) error {
	// Each ilst item contains a 'data' atom.
	return walkAtomBytes(data, func(subName string, payload []byte) error {
		if subName != "data" {
			return nil
		}
		if len(payload) < 8 {
			return nil
		}
		// data atom: 4 bytes type indicator, 4 bytes locale, then value
		typeIndicator := binary.BigEndian.Uint32(payload[:4])
		value := payload[8:]

		switch name {
		case "\xa9nam": // title
			if typeIndicator == 1 {
				p.itunesTitle = string(value)
			}
		case "\xa9alb": // album
			if typeIndicator == 1 {
				p.itunesAlbum = string(value)
			}
		case "aART": // album artist
			if typeIndicator == 1 {
				p.itunesAlbumArtist = string(value)
			}
		case "\xa9ART": // artist
			if typeIndicator == 1 {
				p.itunesArtists = append(p.itunesArtists, string(value))
			}
		case "\xa9day": // year/date
			if typeIndicator == 1 {
				p.itunesYear = string(value)
			}
		case "\xa9cmt", "desc", "\xa9des": // description/comment
			if typeIndicator == 1 && p.itunesDescription == "" {
				p.itunesDescription = string(value)
			}
		case "cprt", "\xa9pub": // publisher/copyright
			if typeIndicator == 1 && p.itunesPublisher == "" {
				p.itunesPublisher = string(value)
			}
		case "covr": // cover art
			if len(value) > 0 && len(p.coverData) == 0 {
				p.coverData = make([]byte, len(value))
				copy(p.coverData, value)
				// type 13 = JPEG, type 14 = PNG
				if typeIndicator == 14 {
					p.coverExt = ".png"
				} else {
					p.coverExt = ".jpg"
				}
			}
		}
		return nil
	})
}

// parseChpl parses Nero chapter data (chpl atom).
func (p *mp4Parser) parseChpl(data []byte) {
	// Format: version(1) + flags(3) + reserved(4) + chapter_count(4) + chapters...
	if len(data) < 9 {
		return
	}
	offset := 8 // skip version(1)+flags(3)+reserved(4)
	if len(data) <= offset {
		return
	}
	count := int(data[offset])
	offset++
	if count == 0 && len(data) > offset+3 {
		// Some encoders write 4-byte count
		count = int(binary.BigEndian.Uint32(data[offset-1 : offset+3]))
		offset += 3
	}

	for i := 0; i < count; i++ {
		if offset+9 > len(data) {
			break
		}
		startMs := binary.BigEndian.Uint64(data[offset : offset+8])
		offset += 8
		titleLen := int(data[offset])
		offset++
		if offset+titleLen > len(data) {
			break
		}
		title := string(data[offset : offset+titleLen])
		offset += titleLen

		p.chapters = append(p.chapters, AudioChapter{
			Title:        title,
			StartSeconds: float64(startMs) / 1000.0,
			Index:        i,
		})
	}

	// Fill in end_seconds for each chapter
	for i := 0; i < len(p.chapters)-1; i++ {
		p.chapters[i].EndSeconds = p.chapters[i+1].StartSeconds
	}
	if len(p.chapters) > 0 && p.durationSeconds > 0 {
		p.chapters[len(p.chapters)-1].EndSeconds = float64(p.durationSeconds)
	}
}

// parseTrak parses a track atom looking for chapter track references.
// For simplicity in Phase 1 we only use Nero chapters (chpl); QuickTime
// chapter tracks (tref/chap) are left for a future enhancement.
func (p *mp4Parser) parseTrak(_ []byte) error {
	return nil
}

// ── Atom walking helpers ────────────────────────────────────────────────────

// walkAtoms walks the top-level atoms of p.r.
func (p *mp4Parser) walkAtoms(start, end int64, fn func(string, []byte) error) error {
	if _, err := p.r.Seek(start, io.SeekStart); err != nil {
		return err
	}
	for {
		var sizeBuf [4]byte
		if _, err := io.ReadFull(p.r, sizeBuf[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return nil
			}
			return err
		}
		size := int64(binary.BigEndian.Uint32(sizeBuf[:]))

		var nameBuf [4]byte
		if _, err := io.ReadFull(p.r, nameBuf[:]); err != nil {
			return err
		}
		name := string(nameBuf[:])

		var payloadSize int64
		if size == 1 {
			// Extended size
			var extSize [8]byte
			if _, err := io.ReadFull(p.r, extSize[:]); err != nil {
				return err
			}
			size = int64(binary.BigEndian.Uint64(extSize[:]))
			payloadSize = size - 16
		} else if size == 0 {
			// Atom extends to EOF — payloadSize stays -1 (skip reading)
			payloadSize = -1
		} else {
			payloadSize = size - 8
		}

		var payload []byte
		if payloadSize > 0 {
			if payloadSize > 64*1024*1024 { // skip atoms > 64 MB
				if _, err := p.r.Seek(payloadSize, io.SeekCurrent); err != nil {
					return nil
				}
			} else {
				payload = make([]byte, payloadSize)
				if _, err := io.ReadFull(p.r, payload); err != nil {
					return nil
				}
			}
		}

		if err := fn(name, payload); err != nil {
			return err
		}

		if end > 0 {
			cur, err := p.r.Seek(0, io.SeekCurrent)
			if err != nil {
				return fmt.Errorf("seeking current position: %w", err)
			}
			if cur >= end {
				break
			}
		}
	}
	return nil
}

// walkAtomBytes walks atoms within a byte slice.
func walkAtomBytes(data []byte, fn func(string, []byte) error) error {
	offset := 0
	for offset+8 <= len(data) {
		size := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		name := string(data[offset+4 : offset+8])

		if size < 8 {
			break
		}
		end := offset + size
		if end > len(data) {
			end = len(data)
		}
		payload := data[offset+8 : end]

		if err := fn(name, payload); err != nil {
			return err
		}
		offset = end
	}
	return nil
}

// ── ID3 parser ───────────────────────────────────────────────────────────────

// id3Parser reads basic ID3v2 tags from an MP3 file.
type id3Parser struct {
	r           *os.File
	title       string
	album       string
	artist      string
	albumArtist string
	year        string
	description string
	coverData   []byte
	coverExt    string
}

func (p *id3Parser) parse() error {
	// Read ID3v2 header (10 bytes)
	var header [10]byte
	if _, err := io.ReadFull(p.r, header[:]); err != nil {
		return fmt.Errorf("reading ID3 header: %w", err)
	}

	if string(header[:3]) != "ID3" {
		return fmt.Errorf("not an ID3v2 file")
	}

	ver := header[3]

	// Sync-safe size
	sz := (int(header[6]) << 21) | (int(header[7]) << 14) | (int(header[8]) << 7) | int(header[9])
	if sz > 50*1024*1024 {
		return fmt.Errorf("ID3 tag too large")
	}

	tagData := make([]byte, sz)
	if _, err := io.ReadFull(p.r, tagData); err != nil {
		return fmt.Errorf("reading ID3 tag data: %w", err)
	}

	return p.parseFrames(ver, tagData)
}

func (p *id3Parser) parseFrames(ver byte, data []byte) error {
	offset := 0
	for offset+10 <= len(data) {
		frameID := string(data[offset : offset+4])
		if frameID == "\x00\x00\x00\x00" || frameID == "" {
			break
		}

		var frameSize int
		if ver == 4 {
			// ID3v2.4: sync-safe integers
			frameSize = (int(data[offset+4]) << 21) | (int(data[offset+5]) << 14) |
				(int(data[offset+6]) << 7) | int(data[offset+7])
		} else {
			// ID3v2.3 and earlier: normal big-endian
			frameSize = (int(data[offset+4]) << 24) | (int(data[offset+5]) << 16) |
				(int(data[offset+6]) << 8) | int(data[offset+7])
		}
		// flags := data[offset+8 : offset+10]  // not used
		offset += 10

		if frameSize <= 0 || offset+frameSize > len(data) {
			break
		}
		payload := data[offset : offset+frameSize]
		offset += frameSize

		p.handleFrame(frameID, payload)
	}
	return nil
}

func (p *id3Parser) handleFrame(id string, data []byte) {
	if len(data) == 0 {
		return
	}

	readText := func() string {
		if len(data) < 2 {
			return ""
		}
		encoding := data[0]
		raw := data[1:]
		switch encoding {
		case 1, 2: // UTF-16
			return decodeUTF16(raw)
		default: // ISO-8859-1 or UTF-8
			return strings.TrimRight(string(raw), "\x00")
		}
	}

	switch id {
	case "TIT2":
		p.title = readText()
	case "TALB":
		p.album = readText()
	case "TPE1":
		p.artist = readText()
	case "TPE2":
		p.albumArtist = readText()
	case "TYER", "TDRC":
		p.year = readText()
	case "COMM":
		// COMM: encoding(1) + language(3) + short desc + \x00 + text
		if len(data) > 4 {
			// skip encoding and language
			rest := data[4:]
			null := strings.IndexByte(string(rest), 0)
			if null >= 0 && null+1 < len(rest) {
				p.description = string(rest[null+1:])
			}
		}
	case "APIC":
		if len(p.coverData) > 0 {
			return
		}
		// encoding(1) + mime(null-term) + picture_type(1) + description(null-term) + data
		if len(data) < 3 {
			return
		}
		mimeEnd := strings.IndexByte(string(data[1:]), 0)
		if mimeEnd < 0 {
			return
		}
		mime := strings.ToLower(string(data[1 : 1+mimeEnd]))
		rest := data[1+mimeEnd+1+1:] // skip mime null, picture type
		descEnd := strings.IndexByte(string(rest), 0)
		if descEnd < 0 {
			descEnd = 0
		} else {
			descEnd++
		}
		imgData := rest[descEnd:]
		if len(imgData) > 0 {
			p.coverData = make([]byte, len(imgData))
			copy(p.coverData, imgData)
			if strings.Contains(mime, "png") {
				p.coverExt = ".png"
			} else {
				p.coverExt = ".jpg"
			}
		}
	}
}

// decodeUTF16 converts a UTF-16 encoded byte slice to a string.
func decodeUTF16(b []byte) string {
	if len(b) < 2 {
		return ""
	}
	// Check BOM
	var order binary.ByteOrder = binary.LittleEndian
	if b[0] == 0xFE && b[1] == 0xFF {
		order = binary.BigEndian
		b = b[2:]
	} else if b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}

	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	runes := make([]rune, len(b)/2)
	for i := range runes {
		runes[i] = rune(order.Uint16(b[i*2:]))
	}
	// Strip null terminators
	s := string(runes)
	return strings.TrimRight(s, "\x00")
}
