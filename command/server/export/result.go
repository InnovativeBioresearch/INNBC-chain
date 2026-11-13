package export

import "bytes"

type cmdResult struct {
	CommandOutput string `json:"export_result"`
}

func (c *cmdResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[EXPORT SUCCESS]\n")
	/*START ALPHA CHANGE 36 -- avoid string concatenation inside WriteString --*/
	buffer.WriteString(c.CommandOutput)
	buffer.WriteString("\n")
	/*FINISH ALPHA CHANGE 36 -- avoid string concatenation inside WriteString --*/

	return buffer.String()
}
