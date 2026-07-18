package dto

type ChangePasswordReq struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

type LoginReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AddAdminQQReq struct {
	QQ int64 `json:"qq"`
}

type UpdateAdapterConfigReq struct {
	Addr   string  `json:"addr"`
	Port   int     `json:"port"`
	Token  string  `json:"token"`
	Admins []int64 `json:"admins"`
}
