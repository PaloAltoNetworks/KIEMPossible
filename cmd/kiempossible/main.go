package main

import "fmt"

func main() {

	banner := `
	 _  _____ ___ __  __ ___           _ _    _     
	| |/ /_ _| __|  \/  | _ \___ _____(_) |__| |___ 
	| ' < | || _|| |\/| |  _/ _ (_-<_-< | '_ \ / -_)
	|_|\_\___|___|_|  |_|_| \___/__/__/_|_.__/_\___|
                                                                            
`
	fmt.Println(banner)
	Collect()
}

// Check change in binding_collection (added error return for GetSubresources)

// Make changes to Azure (similar to AWS - limits, file usage etc) - check batchUpdateDatabase (changed helper function, ensure it doesn't clahs with Azure)
