package logging

type AuditLogEvent struct {
	Namespace  string
	EntityType string
	EntityName string
	Procedure  string
	UserEmail  string
	Source     string
	Headers    string
	Request    string
	Response   string
	Error      string
	MAUUID     string
	RuntimeEnv string
	CRDDiff    string
	PreCRD     string
	TimeStamp  int64
	Dimension  string
}
