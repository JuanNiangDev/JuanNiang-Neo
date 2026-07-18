package dto

import "JuanNiang-Neo/internal/core/models"

func Model2Resp_AdminQQ(raw []models.AdminQQ) []AdminQQNumbers {
	res := make([]AdminQQNumbers, len(raw))

	for i, item := range raw {
		res[i].CreatedAt = item.CreatedAt
		res[i].ID = item.ID
	}

	return res
}
