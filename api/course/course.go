package course

import (
	"fmt"

	"strings"

	"github.com/json-iterator/go"
	"github.com/kataras/iris"
	"github.com/mohuishou/scuplus-go/api"
	cache "github.com/mohuishou/scuplus-go/cache/lists"
	"github.com/mohuishou/scuplus-go/middleware"
	"github.com/mohuishou/scuplus-go/model"
	"github.com/mohuishou/scuplus-go/util/wechat"
	validator "gopkg.in/go-playground/validator.v9"
)

// MinGradeAll 最少需要多少条统计
const MinGradeAll = 10

// GetParams Get 参数
type GetParams struct {
	CallName string `form:"call_name"` // 点名方式
	ExamType string `form:"exam_type"` // 考核方式
	Task     string `form:"task"`      // 有无作业
	Day      string `form:"day"`       // 周几上课
	Campus   string `form:"campus"`    // 校区
	Order    string `form:"order"`     // 排序
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// GetCourses 获取课程列表
func GetCourses(ctx iris.Context) {
	params := GetParams{}
	ctx.ReadForm(&params)

	rkey := fmt.Sprintf("courses.c%s.e%s.t%s.d%s.ca%s.o%s.p%d.ps%d", params.CallName, params.ExamType, params.Task, params.Day, params.Campus, params.Order, params.Page, params.PageSize)
	// 获取缓存信息
	data, err := cache.Get(rkey)
	if err == nil {
		ctx.Write(data)
		return
	}

	var courseCounts []model.CourseCount
	scope := model.DB().Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize)
	if params.CallName != "" {
		scope = scope.Where("call_name = ?", params.CallName)
	}
	if params.Day != "" {
		scope = scope.Where("day = ?", params.Day)
	}
	if params.ExamType != "" {
		scope = scope.Where("exam_type = ?", params.ExamType)
	}
	if params.Task != "" {
		scope = scope.Where("task = ?", params.Task)
	}
	if params.Campus != "" {
		scope = scope.Where("campus = ?", params.Campus)
	}

	if params.Order == "" {
		params.Order = "avg_grade desc"
	}
	scope = scope.Order(params.Order)
	if strings.Contains(params.Order, "avg_grade") {
		scope = scope.Where("grade_all > ?", MinGradeAll)
	}

	if err := scope.Find(&courseCounts).Error; err != nil {
		api.Error(ctx, 70001, "获取错误", nil)
		return
	}
	api.Success(ctx, "获取成功！", courseCounts)
	// 缓存数据,缓存12小时
	cache.Set(rkey, map[string]interface{}{
		"status": 0,
		"msg":    "获取成功！",
		"data":   courseCounts,
	}, 3600*12)
}

// SearchParams Get 参数
type SearchParams struct {
	Name     string `form:"name"` // 搜索的课程名
	Page     int    `form:"page"`
	PageSize int    `form:"page_size"`
}

// Search 课程搜索
func Search(ctx iris.Context) {
	params := SearchParams{}
	ctx.ReadForm(&params)
	if params.Name == "" {
		api.Error(ctx, 70400, "参数错误", nil)
		return
	}
	var courseCounts []model.CourseCount
	scope := model.DB().Offset((params.Page - 1) * params.PageSize).Limit(params.PageSize).Order("avg_grade desc")
	if err := scope.Where("name like ?", "%"+params.Name+"%").Find(&courseCounts).Error; err != nil {
		api.Error(ctx, 70001, "获取错误", nil)
		return
	}
	api.Success(ctx, "获取成功！", courseCounts)
}

// Get 获取一门课程的所有信息
// 包括CourseCount\Cousre\CousreEva中的所有信息
func Get(ctx iris.Context) {
	courseID := ctx.URLParam("course_id")
	lessonID := ctx.URLParam("lesson_id")
	if courseID == "" || lessonID == "" {
		api.Error(ctx, 70400, "参数错误", nil)
		return
	}

	// 获取课程信息
	var (
		courseCount     model.CourseCount
		courses         []model.Course
		courseEvaluates []model.CourseEvaluate
		courseGrades    []model.CourseGrade
	)
	scope := model.DB().Where("course_id = ? and lesson_id = ?", courseID, lessonID)
	scope.Find(&courseCount)
	scope.Find(&courses)
	scope.Find(&courseGrades)

	// todo: 获取用户昵称，用户头像,用户是否已经点赞
	scope.Find(&courseEvaluates)

	// 获取用户是否有该门课程
	uid := middleware.GetUserID(ctx)
	has := !model.DB().Where("user_id = ? and course_id = ? and lesson_id = ?", uid, courseID, lessonID).Select([]string{"id"}).Find(&model.Schedule{}).RecordNotFound()

	// 获取用户是否已经评价
	evaluate := model.CourseEvaluate{}
	model.DB().Where("user_id = ? and course_id = ? and lesson_id = ?", uid, courseID, lessonID).Select([]string{"id"}).Find(&evaluate)

	// 返回成功信息
	api.Success(ctx, "获取成功！", map[string]interface{}{
		"course_count":     courseCount,
		"courses":          courses,
		"course_evaluates": courseEvaluates,
		"course_grades":    courseGrades,
		"has":              has,      // true: 拥有该门课程, false: 不拥有
		"evaluate":         evaluate, // 是否已经评价
	})
}

// CommentParam 课程评价参数
type CommentParam struct {
	ID       uint   `form:"id"`
	CallName int    `form:"call_name" validate:"required,min=1,max=4"` // 点名方式
	ExamType int    `form:"exam_type" validate:"required,min=1,max=4"` // 考核方式
	Task     int    `form:"task" validate:"required,min=1,max=2"`      // 有无作业
	Star     int    `form:"star" validate:"required,min=1,max=3"`
	CourseID string `form:"course_id" validate:"required"`
	LessonID string `form:"lesson_id" validate:"required"`
	Comment  string `form:"comment" validate:"required,min=1,max=200"`
	NickName string `form:"nick_name"`
	Avatar   string `form:"avatar"`
}

// Comment 课程评价，目前只能评价正在上的课程
func Comment(ctx iris.Context) {
	courseEvaluate := commentParam(ctx)
	if courseEvaluate == nil {
		return
	}
	courseEvaluate.Score = 1

	// 获取用户是否有该门课程
	uid := middleware.GetUserID(ctx)
	hasRecord := model.DB().Where("user_id = ? and course_id = ? and lesson_id = ?", uid, courseEvaluate.CourseID, courseEvaluate.LessonID).Select([]string{"id"})
	if hasRecord.Find(&model.Schedule{}).RecordNotFound() {
		api.Error(ctx, 70401, "您的课程表没有该课程！", nil)
		return
	}

	// 检查用户是否已经评价过
	if !hasRecord.Find(&model.CourseEvaluate{}).RecordNotFound() {
		api.Error(ctx, 70401, "该课程您已评价！", nil)
		return
	}

	if err := model.DB().Create(courseEvaluate).Error; err != nil {
		api.Error(ctx, 70002, "评教失败！", err)
		return
	}
	api.Success(ctx, "评教成功！", nil)
}

// UpdateComment 更新评价
func UpdateComment(ctx iris.Context) {
	courseEvaluate := commentParam(ctx)
	if courseEvaluate == nil {
		return
	}

	if courseEvaluate.ID == 0 {
		api.Error(ctx, 70400, "参数错误！", nil)
		return
	}

	if err := model.DB().Model(&model.CourseEvaluate{
		Model: model.Model{ID: courseEvaluate.ID},
	}).Updates(courseEvaluate).Error; err != nil {
		api.Error(ctx, 70002, "更新失败！", err)
		return
	}
	api.Success(ctx, "更新成功！", nil)
}

func commentParam(ctx iris.Context) *model.CourseEvaluate {
	params := CommentParam{}
	if err := ctx.ReadForm(&params); err != nil {
		api.Error(ctx, 70400, "参数错误！", err)
		return nil
	}

	validate := validator.New()
	if err := validate.Struct(params); err != nil {
		api.Error(ctx, 70400, "参数错误！", err.Error())
		return nil
	}

	// 内容安全检查
	b, _ := jsoniter.Marshal(&params)
	res, err := wechat.MsgCheck(string(b))
	if !res {
		api.Error(ctx, 70005, "包含违法违规内容！", err)
		return nil
	}

	return &model.CourseEvaluate{
		Model:    model.Model{ID: params.ID},
		CallName: params.CallName,
		ExamType: params.ExamType,
		Task:     params.Task,
		CourseID: params.CourseID,
		LessonID: params.LessonID,
		Comment:  params.Comment,
		UserID:   middleware.GetUserID(ctx),
		Star:     params.Star,
		Avatar:   params.Avatar,
		NickName: params.NickName,
	}
}

// GetComment 获取已经评价的课程
func GetComment(ctx iris.Context) {
	id, err := ctx.URLParamInt("id")
	if id == 0 || err != nil {
		api.Error(ctx, 70400, "参数错误！", err)
		return
	}
	courseEvaluate := model.CourseEvaluate{}
	if err := model.DB().Find(&courseEvaluate, id).Error; err != nil {
		api.Error(ctx, 70003, "获取失败！", err)
		return
	}

	if courseEvaluate.UserID != middleware.GetUserID(ctx) {
		api.Error(ctx, 70401, "您没有这个权限！", err)
		return
	}

	api.Success(ctx, "获取成功！", courseEvaluate)
}
