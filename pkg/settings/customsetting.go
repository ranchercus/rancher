package settings

//Env var "CATTLE_" + strings.ToUpper(strings.Replace(name, "-", "_", -1))
var (
	PipelineRegistryInsecure = NewSetting("pipeline-registry-insecure", "false")
)
