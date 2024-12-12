package logging

type UAPIAuditLogEvent struct {
	Namespace  *string `heatpipe:"namespace"`
	EntityType *string `heatpipe:"entity_type"`
	EntityName *string `heatpipe:"entity_name"`
	Procedure  *string `heatpipe:"procedure"`
	UserEmail  *string `heatpipe:"user_email"`
	Source     *string `heatpipe:"source"`
	Headers    *string `heatpipe:"headers"`
	Request    *string `heatpipe:"request"`
	Response   *string `heatpipe:"response"`
	Error      *string `heatpipe:"error"`
	MAUUID     *string `heatpipe:"ma_uuid"`
	RuntimeEnv *string `heatpipe:"runtime_env"`
	CRDDiff    *string `heatpipe:"crd_diff"`
	PreCRD     *string `heatpipe:"previous_crd"`
	TimeStamp  *int64  `heatpipe:"timestamp"`
	Dimension  *string `heatpipe:"dimensions"`
}
