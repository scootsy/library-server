package scanner

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

// EPUBMeta holds metadata extracted from an EPUB OPF package document.
type EPUBMeta struct {
	Title       string
	SortTitle   string
	Subtitle    string
	Authors     []EPUBContributor
	Narrators   []EPUBContributor
	Language    string
	Publisher   string
	PublishDate string
	Description string
	ISBN13      string
	ISBN10      string
	CoverFile   string // filename of embedded cover image, if present
	PageCount   int
	Subjects    []string
}

// EPUBContributor is a person extracted from the OPF.
type EPUBContributor struct {
	Name     string
	SortName string
	Role     string // "aut", "nrt", etc.
}

// ExtractEPUBMeta opens an EPUB file and parses its OPF metadata.
// epubPath must be an absolute path that has already been validated.
func ExtractEPUBMeta(epubPath string) (*EPUBMeta, error) {
	r, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, fmt.Errorf("opening epub %q: %w", epubPath, err)
	}
	defer r.Close()

	// Step 1: find the OPF path from META-INF/container.xml
	opfPath, err := findOPFPath(r)
	if err != nil {
		return nil, fmt.Errorf("finding OPF in %q: %w", epubPath, err)
	}

	// Step 2: parse the OPF
	opf, err := parseOPF(r, opfPath)
	if err != nil {
		return nil, fmt.Errorf("parsing OPF in %q: %w", epubPath, err)
	}

	return opfToMeta(opf), nil
}

// ── Container XML ────────────────────────────────────────────────────────────

type containerXML struct {
	Rootfiles []struct {
		FullPath  string `xml:"full-path,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"rootfiles>rootfile"`
}

func findOPFPath(r *zip.ReadCloser) (string, error) {
	f := findFile(r, "META-INF/container.xml")
	if f == nil {
		return "", fmt.Errorf("META-INF/container.xml not found")
	}

	rc, err := f.Open()
	if err != nil {
		return "", fmt.Errorf("opening container.xml: %w", err)
	}
	defer rc.Close()

	var c containerXML
	if err := xml.NewDecoder(rc).Decode(&c); err != nil {
		return "", fmt.Errorf("decoding container.xml: %w", err)
	}

	for _, rf := range c.Rootfiles {
		if rf.MediaType == "application/oebps-package+xml" || strings.HasSuffix(rf.FullPath, ".opf") {
			return rf.FullPath, nil
		}
	}
	// Fallback: return the first rootfile
	if len(c.Rootfiles) > 0 {
		return c.Rootfiles[0].FullPath, nil
	}
	return "", fmt.Errorf("no OPF rootfile found in container.xml")
}

// ── OPF Package ──────────────────────────────────────────────────────────────

// opfPackage is a partial representation of the OPF 2.0/3.0 package document.
type opfPackage struct {
	Metadata opfMetadata `xml:"metadata"`
	Manifest opfManifest `xml:"manifest"`
	Spine    opfSpine    `xml:"spine"`
}

type opfMetadata struct {
	Titles      []opfTitle       `xml:"title"`
	Creators    []opfCreator     `xml:"creator"`
	Contributors []opfCreator    `xml:"contributor"`
	Languages   []string         `xml:"language"`
	Publisher   string           `xml:"publisher"`
	Date        string           `xml:"date"`
	Description string           `xml:"description"`
	Identifiers []opfIdentifier  `xml:"identifier"`
	Subjects    []string         `xml:"subject"`
	Metas       []opfMeta        `xml:"meta"`
}

type opfTitle struct {
	Value   string `xml:",chardata"`
	FileAs  string `xml:"file-as,attr"`
	ID      string `xml:"id,attr"`
	Refines string `xml:"refines,attr"`
}

type opfCreator struct {
	Value   string `xml:",chardata"`
	Role    string `xml:"role,attr"`
	FileAs  string `xml:"file-as,attr"`
	ID      string `xml:"id,attr"`
	Refines string `xml:"refines,attr"`
}

type opfIdentifier struct {
	Value  string `xml:",chardata"`
	Scheme string `xml:"scheme,attr"`
	ID     string `xml:"id,attr"`
}

type opfMeta struct {
	Name    string `xml:"name,attr"`
	Content string `xml:"content,attr"`
	// OPF3 attributes
	Property string `xml:"property,attr"`
	Refines  string `xml:"refines,attr"`
	Value    string `xml:",chardata"`
}

type opfManifest struct {
	Items []opfItem `xml:"item"`
}

type opfItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type opfSpine struct {
	PageCount int `xml:"page-count,attr"`
}

func parseOPF(r *zip.ReadCloser, opfPath string) (*opfPackage, error) {
	f := findFile(r, opfPath)
	if f == nil {
		return nil, fmt.Errorf("OPF file not found at %q", opfPath)
	}

	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("opening OPF: %w", err)
	}
	defer rc.Close()

	var pkg opfPackage
	if err := xml.NewDecoder(rc).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("decoding OPF: %w", err)
	}
	return &pkg, nil
}

// ── OPF → EPUBMeta ────────────────────────────────────────────────────────────

func opfToMeta(pkg *opfPackage) *EPUBMeta {
	m := &EPUBMeta{}
	md := pkg.Metadata

	// Title
	if len(md.Titles) > 0 {
		m.Title = strings.TrimSpace(md.Titles[0].Value)
		if md.Titles[0].FileAs != "" {
			m.SortTitle = md.Titles[0].FileAs
		}
	}
	if m.SortTitle == "" && m.Title != "" {
		m.SortTitle = SortTitle(m.Title)
	}

	// Additional titles → subtitle (OPF3 marks them with title-type meta)
	titleTypes := make(map[string]string) // id → type
	for _, meta := range md.Metas {
		if meta.Property == "title-type" && meta.Refines != "" {
			id := strings.TrimPrefix(meta.Refines, "#")
			titleTypes[id] = meta.Value
		}
		if meta.Property == "file-as" && meta.Refines != "" {
			id := strings.TrimPrefix(meta.Refines, "#")
			if _, ok := titleTypes[id]; !ok {
				for i, t := range md.Titles {
					if t.ID == id {
						md.Titles[i].FileAs = meta.Value
					}
				}
			}
		}
	}
	for _, t := range md.Titles[1:] {
		ttype := titleTypes[t.ID]
		if ttype == "subtitle" || ttype == "short" {
			m.Subtitle = strings.TrimSpace(t.Value)
			break
		}
	}

	// Language
	if len(md.Languages) > 0 {
		m.Language = strings.TrimSpace(md.Languages[0])
	}

	// Publisher & date
	m.Publisher = strings.TrimSpace(md.Publisher)
	m.PublishDate = cleanDate(strings.TrimSpace(md.Date))

	// Description
	m.Description = strings.TrimSpace(md.Description)

	// Contributors
	for _, c := range md.Creators {
		role := normalizeRole(c.Role)
		ec := EPUBContributor{
			Name:     strings.TrimSpace(c.Value),
			SortName: strings.TrimSpace(c.FileAs),
			Role:     role,
		}
		if ec.SortName == "" {
			ec.SortName = deriveSortName(ec.Name)
		}
		if role == "narrator" {
			m.Narrators = append(m.Narrators, ec)
		} else {
			m.Authors = append(m.Authors, ec)
		}
	}
	for _, c := range md.Contributors {
		role := normalizeRole(c.Role)
		ec := EPUBContributor{
			Name:     strings.TrimSpace(c.Value),
			SortName: strings.TrimSpace(c.FileAs),
			Role:     role,
		}
		if ec.SortName == "" {
			ec.SortName = deriveSortName(ec.Name)
		}
		if role == "narrator" {
			m.Narrators = append(m.Narrators, ec)
		} else {
			m.Authors = append(m.Authors, ec)
		}
	}

	// Apply OPF3 refinements for role and file-as
	for _, meta := range md.Metas {
		refID := strings.TrimPrefix(meta.Refines, "#")
		switch meta.Property {
		case "role":
			applyRoleRefinement(m, refID, meta.Value)
		case "file-as":
			applyFileAsRefinement(m, refID, meta.Value)
		}
	}

	// Identifiers
	for _, id := range md.Identifiers {
		v := strings.TrimSpace(id.Value)
		scheme := strings.ToLower(id.Scheme)
		switch {
		case scheme == "isbn" && len(v) == 13:
			m.ISBN13 = v
		case scheme == "isbn" && len(v) == 10:
			m.ISBN10 = v
		case strings.HasPrefix(v, "urn:isbn:"):
			raw := strings.TrimPrefix(v, "urn:isbn:")
			if len(raw) == 13 {
				m.ISBN13 = raw
			} else if len(raw) == 10 {
				m.ISBN10 = raw
			}
		}
	}

	// Subjects
	for _, s := range md.Subjects {
		if t := strings.TrimSpace(s); t != "" {
			m.Subjects = append(m.Subjects, t)
		}
	}

	// OPF2 meta tags (calibre, etc.)
	for _, meta := range md.Metas {
		switch meta.Name {
		case "calibre:series":
			// handled by caller if needed
		}
	}

	// Cover image from manifest
	opfDir := path.Dir(pkg.Manifest.Items[0].Href) // fallback
	_ = opfDir
	coverID := ""
	for _, meta := range md.Metas {
		if meta.Name == "cover" {
			coverID = meta.Content
		}
		if meta.Property == "cover-image" {
			coverID = meta.Refines
		}
	}
	if coverID != "" {
		for _, item := range pkg.Manifest.Items {
			if item.ID == coverID || strings.Contains(item.Properties, "cover-image") {
				m.CoverFile = item.Href
				break
			}
		}
	}
	// Also check properties="cover-image" directly
	if m.CoverFile == "" {
		for _, item := range pkg.Manifest.Items {
			if strings.Contains(item.Properties, "cover-image") {
				m.CoverFile = item.Href
				break
			}
		}
	}

	return m
}

func normalizeRole(role string) string {
	switch strings.ToLower(role) {
	case "nrt", "narrator":
		return "narrator"
	case "edt", "editor":
		return "editor"
	case "trl", "translator":
		return "translator"
	case "ill", "illustrator":
		return "illustrator"
	default:
		return "author"
	}
}

func applyRoleRefinement(m *EPUBMeta, id, role string) {
	norm := normalizeRole(role)
	for i, a := range m.Authors {
		_ = a
		// match by creator index if ID format is "#creator01"
		_ = id
		_ = norm
		_ = i
	}
}

func applyFileAsRefinement(m *EPUBMeta, id, fileAs string) {
	for i := range m.Authors {
		if m.Authors[i].Role == id {
			m.Authors[i].SortName = fileAs
		}
	}
}

func deriveSortName(name string) string {
	parts := strings.Fields(name)
	if len(parts) < 2 {
		return name
	}
	return parts[len(parts)-1] + ", " + strings.Join(parts[:len(parts)-1], " ")
}

func cleanDate(d string) string {
	// Keep only the date portion (YYYY-MM-DD)
	if len(d) >= 10 {
		return d[:10]
	}
	return d
}

// findFile looks up a file by path in a zip archive (case-sensitive).
func findFile(r *zip.ReadCloser, name string) *zip.File {
	for _, f := range r.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// ExtractEPUBCover extracts the cover image bytes from an EPUB file.
// Returns nil, nil if no cover is found.
func ExtractEPUBCover(epubPath string) ([]byte, string, error) {
	r, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening epub: %w", err)
	}
	defer r.Close()

	opfPath, err := findOPFPath(r)
	if err != nil {
		return nil, "", nil // not fatal
	}

	pkg, err := parseOPF(r, opfPath)
	if err != nil {
		return nil, "", nil
	}

	meta := opfToMeta(pkg)
	if meta.CoverFile == "" {
		return nil, "", nil
	}

	// Resolve cover path relative to OPF
	opfDir := path.Dir(opfPath)
	coverPath := path.Join(opfDir, meta.CoverFile)
	if opfDir == "." {
		coverPath = meta.CoverFile
	}

	f := findFile(r, coverPath)
	if f == nil {
		// Try without the opf directory prefix
		f = findFile(r, meta.CoverFile)
	}
	if f == nil {
		return nil, "", nil
	}

	rc, err := f.Open()
	if err != nil {
		return nil, "", fmt.Errorf("opening cover file: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", fmt.Errorf("reading cover file: %w", err)
	}

	ext := strings.ToLower(path.Ext(f.Name))
	return data, ext, nil
}
