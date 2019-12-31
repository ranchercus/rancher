package harbor

type Role int

const (
	ADMIN_ROLE    Role = 1
	DEVELPER_ROLE Role = 2
	GUEST_ROLE    Role = 3
)

type projectMemberOp int

const (
	project_member_op_update = 1
	project_member_op_delete = 2
)

type User struct {
	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	RealName string `json:"realname"`
	Password string `json:"password"`
}

type Project struct {
	ProjectId int `json:"project_id"`
}

type ProjectMember struct {
	Id        int `json:"id"`
	ProjectId int `json:"project_id"`
}
