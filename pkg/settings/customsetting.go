package settings

var (
	PipelinePluginBuildFromRegistry = NewSetting("plugin-build-from-registry", "docker.io.i.fbank.com")
	DefaultOfficialRegistry         = NewSetting("default-official-registry", "registry.docker.i.fbank.com")
)
