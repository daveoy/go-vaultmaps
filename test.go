package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v2"
)

func main() {
	mystrings := "mysql.ro.password=fake-password,mysql.rw.password=fake-password" // pragma: whitelist-secret
	myoutput := map[string]interface{}{}
	for _, value := range strings.Split(mystrings, ",") {
		splitty := strings.Split(value, "=")
		splittwo := strings.Split(splitty[0], ".")
		var myvalue string
		ptr := myoutput
		for _, val := range splittwo {
			if _, ok := ptr[val]; !ok {
				ptr[val] = map[string]interface{}{}
			}
			// Advance the map pointer deeper into the map.
			ptr = ptr[val].(map[string]interface{})
			myvalue = val
		}
		// should have made all the nodes along the way by now
		ptr[myvalue] = splitty[1]
	}
	b, err := yaml.Marshal(myoutput)
	if err != nil {
		fmt.Println("error:", err)
	}
	fmt.Print(string(b))
}
