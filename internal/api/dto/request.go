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
	QQ string `json:"qq"`
}

type UpdateAdapterConfigReq struct {
	Addr           string   `json:"addr"`
	Port           int      `json:"port"`
	Token          string   `json:"token"`
	AdminQQNumbers []string `json:"admin_qq_numbers"`
	Enabled        bool     `json:"enabled"`
}
