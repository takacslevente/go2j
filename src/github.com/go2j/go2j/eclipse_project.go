package main

import (
	"fmt"
	"io/ioutil"
	"strings"
)

var dotProjectTempl = `<?xml version="1.0" encoding="UTF-8"?>
<projectDescription>
        <name>{{projectName}}</name>
        <comment></comment>
        <projects>
        </projects>
        <buildSpec>
                <buildCommand>
                        <name>org.eclipse.jdt.core.javabuilder</name>
                        <arguments>
                        </arguments>
                </buildCommand>
        </buildSpec>
        <natures>
                <nature>org.eclipse.jdt.core.javanature</nature>
        </natures>
</projectDescription>
`
var dotClasspathTempl = `<?xml version="1.0" encoding="UTF-8"?>
<classpath>
        <classpathentry kind="src" path="src"/>
        <classpathentry kind="con" path="org.eclipse.jdt.launching.JRE_CONTAINER"/>
        <classpathentry kind="output" path="bin"/>
</classpath>
`

func generateProject(projectRootPath, projectName string) {
	dotProject := strings.Replace(dotProjectTempl, "{{projectName}}", projectName, -1)
	fmt.Println("add:", projectRootPath+"/.project")
	ioutil.WriteFile(projectRootPath+"/.project", []byte(dotProject), 0644)
	fmt.Println("add:", projectRootPath+"/.classpath")
	ioutil.WriteFile(projectRootPath+"/.classpath", []byte(dotClasspathTempl), 0644)
}
