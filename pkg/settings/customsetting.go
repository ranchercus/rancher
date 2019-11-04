package settings

//Env var "CATTLE_" + strings.ToUpper(strings.Replace(name, "-", "_", -1))
var (
	PipelineRegistryInsecure = NewSetting("pipeline-registry-insecure", "false")
	PipelineDefaultRegistry = NewSetting("pipeline-default-registry", "")
	PipelineShellDir = NewSetting("pipeline-shell-dir", "")
	PipelineShellName = NewSetting("pipeline-shell-name", "")
	PipelineNodeToleration = NewSetting("pipeline-node-toleration", "")
	PipelineNodeSelector = NewSetting("pipeline-node-selector", "")
	PipelineLocalShare = NewSetting("pipeline-local-share", "")
)
