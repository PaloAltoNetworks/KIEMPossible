package main

import (
	"fmt"

	"github.com/Golansami125/kiempossible/pkg/auth_handling"
)

func main() {
	banner := `
	 _  _____ ___ __  __ ___           _ _    _     
	| |/ /_ _| __|  \/  | _ \___ _____(_) |__| |___ 
	| ' < | || _|| |\/| |  _/ _ (_-<_-< | '_ \ / -_)
	|_|\_\___|___|_|  |_|_| \___/__/__/_|_.__/_\___|
                                                                            
`
	fmt.Println(banner)
	credPath, _, _ := auth_handling.Authenticator()
	Collect()
	if credPath.ShouldAdvise {
		Advise()
	}
}

// TODO
// Recheck documentation for workload_identities table and logic
