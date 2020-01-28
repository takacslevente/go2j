package main

import (
	"io/ioutil"
	"os"
	"strings"
	"fmt"
)

func FileList(basePath, trimPrefix, addPrefix string) []string{
	list := []string{}
	files, _ := ioutil.ReadDir(basePath)
	for _, afile := range files {
		childPath := basePath + "/" + afile.Name()

		stat, err := os.Stat(childPath)

		if err != nil {
			fmt.Println("stat failed:", childPath)
			continue
		}
		isFolder := stat.IsDir()
		if isFolder {
			list = append(list, FileList(childPath, trimPrefix, addPrefix)...)
			continue
		}

		if addPrefix != "" {
			childPath = addPrefix + childPath
		} else {
			childPath = strings.TrimPrefix(childPath, trimPrefix)
		} 
		list = append(list, childPath)
	}
	return list
}
