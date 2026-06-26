package handler

import (
	"database/sql"
	"fmt"
	"groupbuy/model"
	"groupbuy/response"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type CreateGroupReq struct {
	Title      string `json:"title" binding:"required"`
	CutoffTime string `json:"cutoff_time" binding:"required"`
}

func CreateGroup(c *gin.Context) {
	var req CreateGroupReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeParamError)
		return
	}

	cutoff, err := parseTime(req.CutoffTime)
	if err != nil {
		response.FailWithMsg(c, response.CodeParamError, "截团时间格式错误，示例: 2026-06-30 18:00:00")
		return
	}

	if cutoff.Before(time.Now()) {
		response.FailWithMsg(c, response.CodeParamError, "截团时间不能早于当前时间")
		return
	}

	res, err := model.DB.Exec(
		"INSERT INTO group_sessions (title, cutoff_time, status) VALUES (?, ?, 'open')",
		req.Title, cutoff.Format("2006-01-02 15:04:05"),
	)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	id, _ := res.LastInsertId()
	response.OK(c, gin.H{"id": id, "title": req.Title, "cutoff_time": cutoff, "status": "open"})
}

func ListGroups(c *gin.Context) {
	status := c.Query("status")
	query := "SELECT id, title, cutoff_time, status, created_at FROM group_sessions"
	args := []interface{}{}
	if status != "" {
		query += " WHERE status = ?"
		args = append(args, status)
	}
	query += " ORDER BY id DESC"

	rows, err := model.DB.Query(query, args...)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id int64
		var title, cutoffStr, status, createdAt string
		if err := rows.Scan(&id, &title, &cutoffStr, &status, &createdAt); err != nil {
			continue
		}
		cutoff, _ := parseTime(cutoffStr)
		now := time.Now()
		if status == "open" && now.After(cutoff) {
			model.DB.Exec("UPDATE group_sessions SET status = 'closed' WHERE id = ?", id)
			status = "closed"
		}
		list = append(list, gin.H{
			"id": id, "title": title, "cutoff_time": cutoff,
			"status": status, "created_at": createdAt,
		})
	}
	if list == nil {
		list = []gin.H{}
	}
	response.OK(c, list)
}

func GetGroup(c *gin.Context) {
	id := c.Param("id")
	var gid int64
	var title, cutoffStr, status, createdAt string
	err := model.DB.QueryRow(
		"SELECT id, title, cutoff_time, status, created_at FROM group_sessions WHERE id = ?", id,
	).Scan(&gid, &title, &cutoffStr, &status, &createdAt)
	if err == sql.ErrNoRows {
		response.Fail(c, response.CodeNotFound)
		return
	}
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	cutoff, _ := parseTime(cutoffStr)
	now := time.Now()
	if status == "open" && now.After(cutoff) {
		model.DB.Exec("UPDATE group_sessions SET status = 'closed' WHERE id = ?", id)
		status = "closed"
	}
	response.OK(c, gin.H{
		"id": gid, "title": title, "cutoff_time": cutoff,
		"status": status, "created_at": createdAt,
	})
}

func CloseGroup(c *gin.Context) {
	id := c.Param("id")
	res, err := model.DB.Exec("UPDATE group_sessions SET status = 'closed' WHERE id = ? AND status = 'open'", id)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		response.FailWithMsg(c, response.CodeNotFound, "团次不存在或已关闭")
		return
	}
	response.OK(c, gin.H{"id": id, "status": "closed"})
}

type AddProductReq struct {
	Name      string  `json:"name" binding:"required"`
	UnitPrice float64 `json:"unit_price" binding:"required,gt=0"`
	Unit      string  `json:"unit"`
}

func AddProduct(c *gin.Context) {
	groupID := c.Param("id")
	var req AddProductReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeParamError)
		return
	}

	unit := req.Unit
	if unit == "" {
		unit = "份"
	}

	res, err := model.DB.Exec(
		"INSERT INTO products (group_id, name, unit_price, unit, on_shelf) VALUES (?, ?, ?, ?, 1)",
		groupID, req.Name, req.UnitPrice, unit,
	)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}

	pid, _ := res.LastInsertId()
	response.OK(c, gin.H{"id": pid, "group_id": groupID, "name": req.Name, "unit_price": req.UnitPrice, "unit": unit, "on_shelf": true})
}

func ListProducts(c *gin.Context) {
	groupID := c.Param("id")
	shelfOnly := c.Query("on_shelf") == "1"

	query := "SELECT id, group_id, name, unit_price, unit, on_shelf FROM products WHERE group_id = ?"
	args := []interface{}{groupID}
	if shelfOnly {
		query += " AND on_shelf = 1"
	}
	query += " ORDER BY id"

	rows, err := model.DB.Query(query, args...)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	defer rows.Close()

	var list []gin.H
	for rows.Next() {
		var id int64
		var gid int64
		var name, unit string
		var unitPrice float64
		var onShelf int
		if err := rows.Scan(&id, &gid, &name, &unitPrice, &unit, &onShelf); err != nil {
			continue
		}
		list = append(list, gin.H{
			"id": id, "group_id": gid, "name": name,
			"unit_price": unitPrice, "unit": unit, "on_shelf": onShelf == 1,
		})
	}
	if list == nil {
		list = []gin.H{}
	}
	response.OK(c, list)
}

func ToggleProduct(c *gin.Context) {
	productID := c.Param("pid")
	var req struct {
		OnShelf bool `json:"on_shelf"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeParamError)
		return
	}

	onShelf := 0
	if req.OnShelf {
		onShelf = 1
	}

	res, err := model.DB.Exec("UPDATE products SET on_shelf = ? WHERE id = ?", onShelf, productID)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		response.Fail(c, response.CodeNotFound)
		return
	}
	response.OK(c, gin.H{"id": productID, "on_shelf": req.OnShelf})
}

func UpdateProduct(c *gin.Context) {
	productID := c.Param("pid")
	var req struct {
		Name      string  `json:"name"`
		UnitPrice float64 `json:"unit_price"`
		Unit      string  `json:"unit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.CodeParamError)
		return
	}

	sets := []string{}
	args := []interface{}{}
	if req.Name != "" {
		sets = append(sets, "name = ?")
		args = append(args, req.Name)
	}
	if req.UnitPrice > 0 {
		sets = append(sets, "unit_price = ?")
		args = append(args, req.UnitPrice)
	}
	if req.Unit != "" {
		sets = append(sets, "unit = ?")
		args = append(args, req.Unit)
	}
	if len(sets) == 0 {
		response.Fail(c, response.CodeParamError)
		return
	}

	args = append(args, productID)
	query := "UPDATE products SET " + joinSets(sets) + " WHERE id = ?"
	res, err := model.DB.Exec(query, args...)
	if err != nil {
		response.Fail(c, response.CodeInternalError)
		return
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		response.Fail(c, response.CodeNotFound)
		return
	}
	response.OK(c, gin.H{"id": productID})
}

func joinSets(sets []string) string {
	result := ""
	for i, s := range sets {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

func checkCutoff(groupID int64) (bool, string) {
	var cutoffStr, status string
	err := model.DB.QueryRow(
		"SELECT cutoff_time, status FROM group_sessions WHERE id = ?", groupID,
	).Scan(&cutoffStr, &status)
	if err != nil {
		return true, "团次不存在"
	}
	if status == "closed" {
		return true, "团次已关闭"
	}
	cutoff, err := parseTime(cutoffStr)
	if err != nil {
		return true, "截团时间解析失败"
	}
	if time.Now().After(cutoff) {
		model.DB.Exec("UPDATE group_sessions SET status = 'closed' WHERE id = ?", groupID)
		return true, "已过截团时间"
	}
	return false, ""
}

func parseTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
	}
	for _, f := range formats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time: %s", s)
}

func groupExists(groupID int64) bool {
	var id int64
	err := model.DB.QueryRow("SELECT id FROM group_sessions WHERE id = ?", groupID).Scan(&id)
	return err == nil
}

func parseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

func writeJSON(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, data)
}
