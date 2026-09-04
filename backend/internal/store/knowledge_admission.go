package store

// Admission is produced by the versioned cleaner. Recheck the scan/content
// binding on every catalog read so a stale WeKnora ID or cached hit cannot
// silently reuse a decision made against different text.
const documentAdmissionSQL = `
  AND d.admission_status='READY' AND d.admission_version='admission-v1'
  AND d.parse_status='PARSED' AND d.pii_scan_status='CLEAR'
  AND d.content_hash<>'' AND d.pii_content_hash=d.content_hash
  AND d.content_chars BETWEEN 100 AND 100000
  AND d.secondary_topic NOT IN ('','other')
`

const attachmentAdmissionSQL = `
  AND a.admission_status='READY' AND a.parse_status='PARSED'
  AND a.pii_scan_status='CLEAR' AND a.content_hash<>''
  AND a.pii_content_hash=a.content_hash AND a.relation_status='RESOLVED'
`
