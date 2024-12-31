package logging

type AuditLogEvent struct {
	Namespace  string
	EntityType string
	EntityName string
	Procedure  string
	Request    string
	Response   string
	Error      string
	PreCRD     string
}
