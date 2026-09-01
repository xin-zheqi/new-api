package common

import (
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

// Monitor 定时监控cpu使用率，超过阈值输出pprof文件
func Monitor() {
	for {
		percent, err := cpu.Percent(time.Second, false)
		if err != nil {
			panic(err)
		}
		if percent[0] > 80 {
			fmt.Println("cpu usage too high")
			if err := os.MkdirAll("./pprof", 0700); err != nil {
				SysLog("创建pprof文件夹失败 " + err.Error())
				continue
			}
			if err := os.Chmod("./pprof", 0700); err != nil {
				SysLog("设置pprof文件夹权限失败 " + err.Error())
				continue
			}
			profilePath := "./pprof/" + fmt.Sprintf("cpu-%s.pprof", time.Now().Format("20060102150405"))
			f, err := os.OpenFile(profilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
			if err != nil {
				SysLog("创建pprof文件失败 " + err.Error())
				continue
			}
			err = pprof.StartCPUProfile(f)
			if err != nil {
				_ = f.Close()
				SysLog("启动pprof失败 " + err.Error())
				continue
			}
			time.Sleep(10 * time.Second)
			pprof.StopCPUProfile()
			_ = f.Close()
		}
		time.Sleep(30 * time.Second)
	}
}
