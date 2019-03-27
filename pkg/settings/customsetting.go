package settings

//Env var "CATTLE_" + strings.ToUpper(strings.Replace(name, "-", "_", -1))
var (
	DefaultPipelineRegistry  = NewSetting("default-pipeline-registry", "registry.docker.i.fbank.com")
	PipelineRegistryInsecure = NewSetting("pipeline-registry-insecure", "false")
)
