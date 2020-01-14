package main

import (
	"io/ioutil"
	"os"
)

var orgGo2jUtil = `package org.go2j.util;

import java.util.Arrays;

public class ArrayUtil {
	
	public static <T> T[] append(T[] original, T element) {
		T[] copy = Arrays.copyOf(original, original.length + 1);
		copy[original.length] = element;
		return copy;
	}

}
`

func generateHelperClasses(projectSrcPath string) {
	os.MkdirAll(projectSrcPath + "/org/go2j/util", 0755)
	ioutil.WriteFile(projectSrcPath+"/org/go2j/util/ArrayUtil.java", []byte(orgGo2jUtil), 0644)
}
