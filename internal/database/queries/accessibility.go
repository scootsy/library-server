package queries

import (
	"database/sql"
	"fmt"
)

// WorkAccessibility holds EPUB Accessibility 1.1 metadata for a work,
// as required by the EU Accessibility Act (EAA, Directive 2019/882).
//
// Slice fields (AccessModes, Features, etc.) are stored as JSON arrays in TEXT
// columns so they can be queried and filtered by the application layer without
// requiring additional junction tables.
type WorkAccessibility struct {
	WorkID string

	// schema:accessMode — JSON array, e.g. `["textual","visual"]`.
	AccessModes string

	// schema:accessModeSufficient — JSON array of arrays,
	// e.g. `[["textual"],["auditory","textual"]]`.
	AccessModesSufficient string

	// schema:accessibilityFeature — JSON array of feature tokens.
	// Key token for aligned readalouds: "synchronizedAudioText".
	Features string

	// schema:accessibilityHazard — JSON array of hazard tokens.
	Hazards string

	// schema:accessibilitySummary — human-readable prose required by EPUB A11y 1.1.
	Summary string

	// Conformance claim (dcterms:conformsTo / a11y: vocabulary).
	ConformanceStandard string // e.g. "EPUB Accessibility 1.1 - WCAG 2.1 Level AA"
	WCAGLevel           string // "A", "AA", or "AAA"
	WCAGVersion         string // "2.0", "2.1", or "2.2"
	Certifier           string // a11y:certifiedBy
	CertifierCredential string // a11y:certifierCredential URL
	CertifierReport     string // a11y:certifierReport URL
	CertificationDate   string // YYYY-MM-DD

	// Media overlay / aligned readaloud specifics.
	// These complement the file-level fields in SidecarMediaOverlay.
	OverlayNarratorName     string
	OverlayNarratorLanguage string // BCP-47, e.g. "en-US"
	OverlayDurationSeconds  int
	SMILVersion             string // e.g. "3.0"
	SyncGranularity         string // "word" | "sentence" | "paragraph"
	ActiveClass             string // CSS class applied to active text element
	PlaybackActiveClass     string // CSS class applied while playing
}

// UpsertWorkAccessibility inserts or replaces the accessibility record for a work.
func UpsertWorkAccessibility(db *sql.DB, a *WorkAccessibility) error {
	_, err := db.Exec(`
		INSERT INTO work_accessibility (
			work_id,
			access_modes, access_modes_sufficient, features, hazards, summary,
			conformance_standard, wcag_level, wcag_version,
			certifier, certifier_credential, certifier_report, certification_date,
			overlay_narrator_name, overlay_narrator_language, overlay_duration_seconds,
			smil_version, sync_granularity, active_class, playback_active_class
		) VALUES (
			?,
			?, ?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?,
			?, ?, ?, ?
		)
		ON CONFLICT(work_id) DO UPDATE SET
			access_modes               = excluded.access_modes,
			access_modes_sufficient    = excluded.access_modes_sufficient,
			features                   = excluded.features,
			hazards                    = excluded.hazards,
			summary                    = excluded.summary,
			conformance_standard       = excluded.conformance_standard,
			wcag_level                 = excluded.wcag_level,
			wcag_version               = excluded.wcag_version,
			certifier                  = excluded.certifier,
			certifier_credential       = excluded.certifier_credential,
			certifier_report           = excluded.certifier_report,
			certification_date         = excluded.certification_date,
			overlay_narrator_name      = excluded.overlay_narrator_name,
			overlay_narrator_language  = excluded.overlay_narrator_language,
			overlay_duration_seconds   = excluded.overlay_duration_seconds,
			smil_version               = excluded.smil_version,
			sync_granularity           = excluded.sync_granularity,
			active_class               = excluded.active_class,
			playback_active_class      = excluded.playback_active_class
	`,
		a.WorkID,
		nullableString(a.AccessModes), nullableString(a.AccessModesSufficient),
		nullableString(a.Features), nullableString(a.Hazards), nullableString(a.Summary),
		nullableString(a.ConformanceStandard), nullableString(a.WCAGLevel), nullableString(a.WCAGVersion),
		nullableString(a.Certifier), nullableString(a.CertifierCredential),
		nullableString(a.CertifierReport), nullableString(a.CertificationDate),
		nullableString(a.OverlayNarratorName), nullableString(a.OverlayNarratorLanguage),
		nullableInt(a.OverlayDurationSeconds),
		nullableString(a.SMILVersion), nullableString(a.SyncGranularity),
		nullableString(a.ActiveClass), nullableString(a.PlaybackActiveClass),
	)
	if err != nil {
		return fmt.Errorf("upsert work_accessibility for work %s: %w", a.WorkID, err)
	}
	return nil
}

// GetWorkAccessibility returns the accessibility record for workID, or nil if
// none has been stored yet.
func GetWorkAccessibility(db *sql.DB, workID string) (*WorkAccessibility, error) {
	row := db.QueryRow(`
		SELECT
			work_id,
			access_modes, access_modes_sufficient, features, hazards, summary,
			conformance_standard, wcag_level, wcag_version,
			certifier, certifier_credential, certifier_report, certification_date,
			overlay_narrator_name, overlay_narrator_language, overlay_duration_seconds,
			smil_version, sync_granularity, active_class, playback_active_class
		FROM work_accessibility
		WHERE work_id = ?
	`, workID)

	var a WorkAccessibility
	var (
		accessModes, accessModesSufficient, features, hazards, summary sql.NullString
		conformanceStandard, wcagLevel, wcagVersion                    sql.NullString
		certifier, certifierCredential, certifierReport                sql.NullString
		certificationDate                                              sql.NullString
		overlayNarratorName, overlayNarratorLanguage                  sql.NullString
		overlayDurationSeconds                                         sql.NullInt64
		smilVersion, syncGranularity, activeClass, playbackActiveClass sql.NullString
	)

	err := row.Scan(
		&a.WorkID,
		&accessModes, &accessModesSufficient, &features, &hazards, &summary,
		&conformanceStandard, &wcagLevel, &wcagVersion,
		&certifier, &certifierCredential, &certifierReport, &certificationDate,
		&overlayNarratorName, &overlayNarratorLanguage, &overlayDurationSeconds,
		&smilVersion, &syncGranularity, &activeClass, &playbackActiveClass,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get work_accessibility for work %s: %w", workID, err)
	}

	a.AccessModes = accessModes.String
	a.AccessModesSufficient = accessModesSufficient.String
	a.Features = features.String
	a.Hazards = hazards.String
	a.Summary = summary.String
	a.ConformanceStandard = conformanceStandard.String
	a.WCAGLevel = wcagLevel.String
	a.WCAGVersion = wcagVersion.String
	a.Certifier = certifier.String
	a.CertifierCredential = certifierCredential.String
	a.CertifierReport = certifierReport.String
	a.CertificationDate = certificationDate.String
	a.OverlayNarratorName = overlayNarratorName.String
	a.OverlayNarratorLanguage = overlayNarratorLanguage.String
	a.OverlayDurationSeconds = int(overlayDurationSeconds.Int64)
	a.SMILVersion = smilVersion.String
	a.SyncGranularity = syncGranularity.String
	a.ActiveClass = activeClass.String
	a.PlaybackActiveClass = playbackActiveClass.String

	return &a, nil
}
