package harbor

import (
	"fmt"
	"github.com/rancher/norman/types"
	"github.com/rancher/norman/types/convert"
	"github.com/rancher/types/client/cluster/v3"
	"time"
)

const (
	FORMAT_TIME_PATTERN = "2006-01-02 15:04:05"
)

func ProjectFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	createTime := convert.ToString(resource.Values[client.HarborProjectFieldCreationTime])
	t, _ := time.Parse(time.RFC3339Nano, createTime)

	resource.Values[client.HarborProjectFieldCreationTime] = t.In(time.Local).Format(FORMAT_TIME_PATTERN)
}

func TagFormatter(apiContext *types.APIContext, resource *types.RawResource) {
	created := convert.ToString(resource.Values[client.HarborTagFieldCreated])
	size, _ := convert.ToFloat(resource.Values[client.HarborTagFieldSize])

	t, _ := time.Parse(time.RFC3339Nano, created)

	resource.Values[client.HarborTagFieldCreated] = t.In(time.Local).Format(FORMAT_TIME_PATTERN)
	resource.Values[client.HarborTagFieldSize] = fmt.Sprintf("%.2fMB", size / 1048576.0)
}
