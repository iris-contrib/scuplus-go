package config

import (
	"log"
	"os"

	"github.com/BurntSushi/toml"
)

// Mysql 配置
type Mysql struct {
	Host     string
	User     string
	Password string
	DB       string
	Port     string
}

// CourseTask 任务配置文件
type CourseTask struct {
	StudentID int    `toml:"student_id"`
	Password  string `toml:"password"`
	PageNO    int    `toml:"page_no"`
}

type Wechat struct {
	Appid  string `toml:"appid"`
	Secret string `toml:"secret"`
}

// Config 对应config.yml文件的位置
type Config struct {
	Secret     string
	JwtSecret  string `toml:"jwt_secret"`
	Mysql      `toml:"mysql"`
	CourseTask `toml:"course_task"`
	Wechat     `toml:"wechat"`
}

// config
var config Config

// 配置文件路径
var configFile = ""

// Get 获取config
func Get() Config {
	if config.Host == "" {
		// 默认配置文件在同级目录
		filepath := getPath(configFile)

		// 解析配置文件
		if _, err := toml.DecodeFile(filepath, &config); err != nil {
			log.Fatal("配置文件读取失败！", err)
		}
	}
	return config
}

// SetPath 设置Config文件的路径
func SetPath(path string) {
	configFile = path
}

// 获取文件路径
func getPath(path string) string {
	if path != "" {
		return path
	}
	// 获取当前环境
	env := os.Getenv("SCUPLUS_ENV")
	if env == "" {
		env = "develop"
	}

	// 默认配置文件在同级目录
	filepath := "config.toml"

	// 根据环境变量获取配置文件目录
	switch env {
	case "test":
		filepath = os.Getenv("GOPATH") + "/src/github.com/mohuishou/scuplus-go/config/" + filepath
	}
	return filepath
}
