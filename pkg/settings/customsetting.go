package settings

var (
	DefaultDockerRegistry = NewSetting("default-docker-registry", "docker.io.i.fbank.com")
	DefaultPipelineRegistry         = NewSetting("default-pipeline-registry", "registry.docker.i.fbank.com")
)
