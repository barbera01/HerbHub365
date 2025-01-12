package soiltemp


import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func PrintName() {
	fmt.Println("SoilTemp Sensor")
}
 func ReadTemp()  {
		// Path to the devices directory
		devicesPath := "/sys/bus/w1/devices"

		// Get all files matching the pattern */w1_slave
		files, err := filepath.Glob(filepath.Join(devicesPath, "*/w1_slave"))
		if err != nil {
			fmt.Printf("Error finding w1_slave files: %v\n", err)
			os.Exit(1)
		}
	
		if len(files) == 0 {
			fmt.Println("No w1_slave files found")
			return
		}
	
		// Iterate through all matching files
		for _, file := range files {
			data, err := ioutil.ReadFile(file)
			if err != nil {
				fmt.Printf("Error reading file %s: %v\n", file, err)
				continue
			}
	
			// Process the file contents
			content := string(data)
			lines := strings.Split(content, "\n")
			if len(lines) > 1 && strings.Contains(lines[0], "YES") {
				// Extract the temperature from the second line
				tempLine := lines[1]
				tempParts := strings.Split(tempLine, "t=")
				if len(tempParts) > 1 {
					tempStr := tempParts[1]
					fmt.Printf("%s.%s\n", tempStr[:2], tempStr[2:])
				} else {
					fmt.Printf("0.0\n")
				}
			} else {
				fmt.Printf("0.0\n")
			}
		}
	}