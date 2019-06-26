package settings

//Env var "CATTLE_" + strings.ToUpper(strings.Replace(name, "-", "_", -1))
var (
	PipelineRegistryInsecure = NewSetting("pipeline-registry-insecure", "false")
	PipelineDefaultRegistry = NewSetting("pipeline-default-registry", "")
	PipelineFTPServer = NewSetting("pipeline-ftp-server", "")
	PipelineFTPUsername = NewSetting("pipeline-ftp-username", "")
	PipelineFTPPassword = NewSetting("pipeline-ftp-password", "")
)
