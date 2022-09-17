package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

func main() {
	var arg1 string
	//switch statement here avoids any index out of range errors from the argument array.
	switch {
	case len(os.Args[1:]) == 0:
		fmt.Println("to run WebRecon, run the following commands. replace <name> with whatever you like.\n\n1. Create a directory for the test\n\t$ mkdir -p ./Programs/<name>/recon-data\n2. Create a domains.txt file containing the domains to test\n\t $ vim ./Programs/<name>/recon-data/domains.txt\n\nEach domain should be on a newline:\n\tfoo.com\n\tbar.com")
		os.Exit(1)
	case len(os.Args[1:]) == 1:
		arg1 = os.Args[1]
	}
	// get program name as argument
	program_name := arg1
	//get date
	date := time.Now().Format("01-02-2006")

	// make dirs for recon
	prepDirsCommand := "mkdir -p ./Programs/" + program_name + "/" + date
	exec.Command("bash", "-c", prepDirsCommand).Output()
	// create go routine shiz
	var wg sync.WaitGroup
	fmt.Println("Starting Enumeration...")
	//start commonspeak sub generation
	domains_list, err := os.Open("./Programs/" + program_name + "/recon-data/domains.txt")
	if err != nil {
		fmt.Println("Did you create an entry in ./Programs/ dir for " + program_name + "?")
		os.Exit(1)
	}
	defer domains_list.Close()
	scanner := bufio.NewScanner(domains_list)
	scanner.Split(bufio.ScanLines)
	var domains []string
	for scanner.Scan() {
		domains = append(domains, scanner.Text())
	}
	fmt.Println("\nDomains to be tested: ")
	fmt.Println(domains)
	fmt.Print("\n")
	// for file in splits folder
	var files []string
	splits_folder := "./wordlists/commonspeak-splits"
	walkerr := filepath.Walk(splits_folder, func(path string, info os.FileInfo, err error) error {
		files = append(files, path)
		return nil
	})
	if walkerr != nil {
		fmt.Println("error walking folder")
	}
	fmt.Println("Generating potential subdomains. . .")
	for _, split := range files {
		go runCommonspeakGeneration(domains, program_name, split, date, &wg)
		wg.Add(1)
	}

	// run amass
	programpath := "./Programs/" + program_name + "/" + date + "/"
	go RunAmass(program_name, programpath, &wg)
	wg.Add(1)

	// run subfinder
	go RunSubfinder(program_name, programpath, &wg)
	wg.Add(1)
	wg.Wait()

	fmt.Println("subfinder, amass, commonspeak Complete!")

	//run shuffledns to acquire initial list of live hosts.
	programpath3 := "./Programs/" + program_name + "/" + date + "/"
	// combine enumeated subdomains into one file
	CombineSubsCmd := "sort -u " + programpath3 + "subfinder.out " + programpath3 + "amass.out " + programpath3 + "commonspeakresults.out " + " > " + programpath3 + "subdomainscombined.txt"
	exec.Command("bash", "-c", CombineSubsCmd).Output()
	//if err != nil {
	//	fmt.Println("Error Combining subs...")
	//}
	//fmt.Println(string(CombineSubsOut))

	// run shuffledns
	for _, domain := range domains {
		//mkdir for domain
		mkdirCommand := "mkdir -p ./Programs/" + program_name + "/" + date + "/" + domain
		mkdirCommandOut, err := exec.Command("bash", "-c", mkdirCommand).Output()
		if err != nil {
			fmt.Println("Error creating dirs for domains")
		}
		fmt.Println(string(mkdirCommandOut))
		//grep from subdomainscombined using domain
		grepsubsCommand := "cat ./Programs/" + program_name + "/" + date + "/" + "subdomainscombined.txt | grep " + domain + " > ./Programs/" + program_name + "/" + date + "/" + domain + "/subdomains.txt"
		grepsubsCommandOut, err := exec.Command("bash", "-c", grepsubsCommand).Output()
		if err != nil {
			fmt.Println("Error grep subs command")
		}
		fmt.Println(string(grepsubsCommandOut))

		//run shuffledns
		shuffle_output_path := "./Programs/" + program_name + "/" + date + "/" + domain + "/"
		go RunMassdns(program_name, shuffle_output_path, "1", domain, &wg)
		wg.Add(1)
	}
	wg.Wait()
	fmt.Println("shuffledns complete")

	//run dnsgen (generate potential subdomains from already enumerated subdomains)
	for _, domain := range domains {
		fmt.Println("Running dnsgen on " + domain)
		programpath4 := "./Programs/" + program_name + "/" + date + "/" + domain + "/"
		go RunDNSGen(program_name, programpath4, &wg)
		wg.Add(1)
	}
	fmt.Println("Waiting on dnsgen . . .")
	wg.Wait()

	//run shuffledns, mode 2. (resolves subdomains created by dnsgen)
	for _, domain := range domains {
		//run shuffledns
		shuffle_output_path := "./Programs/" + program_name + "/" + date + "/" + domain + "/"
		go RunMassdns(program_name, shuffle_output_path, "2", domain, &wg)
		wg.Add(1)
	}
	fmt.Println("Waiting on shuffledns mode 2...")
	wg.Wait()
	fmt.Println("Complete!")
}

func RunSubfinder(fleetName string, outputPath string, wg *sync.WaitGroup) {
	subFinderCommand := "subfinder -dL ./Programs/" + fleetName + "/recon-data/" + "domains.txt -o " + outputPath + "subfinder.out"
	fmt.Println("Running subfinder - " + subFinderCommand)
	exec.Command("bash", "-c", subFinderCommand).Output()
	//fmt.Println(string(subFinderOut))
	//if err != nil {
	//	fmt.Println("Error Subfinder")
	//}
	wg.Done()
}

func runCommonspeakGeneration(domains []string, program string, blockNum string, date string, wg *sync.WaitGroup) {
	// for line in domains, for line in split, prepend split line to domain, flush to file
	split_file, _ := os.Open("./" + blockNum)
	defer split_file.Close()
	scanner := bufio.NewScanner(split_file)
	scanner.Split(bufio.ScanLines)
	var split_file_lines []string
	for scanner.Scan() {
		split_file_lines = append(split_file_lines, scanner.Text())
	}
	//open output
	output_file, err := os.OpenFile("./Programs/"+program+"/"+date+"/commonspeakresults.out", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("error opening commonspeak results file")
	}
	defer output_file.Close()
	for _, domain := range domains {
		for _, line := range split_file_lines {
			if _, err := output_file.WriteString(line + "." + domain + "\n"); err != nil {
				log.Fatal(err)
			}
		}
	}
	wg.Done()
}

func RunAmass(fleetName string, outputPath string, wg *sync.WaitGroup) {
	// run amass
	RunAmassCommand := "amass enum -passive -timeout 30 -df ./Programs/" + fleetName + "/recon-data/" + "domains.txt | tee -a " + outputPath + "amass.out"
	fmt.Println("Running Amass - " + RunAmassCommand)
	exec.Command("bash", "-c", RunAmassCommand).Output()
	//fmt.Println("amass out: " + string(RunAmassOut))
	//if err != nil {
	//	fmt.Println("Error Amass")
	//}
	//fmt.Println(string(RunAmassOut))
	wg.Done()
}

func RunMassdns(fleetName string, outputPath string, mode string, domain string, wg *sync.WaitGroup) {
	if mode == "1" {
		// run shuffledns in mode 1: runs after initial enumeration.
		// Trying out Shuffledns instead. Loop through domains in domains.txt, mkdir for each domainm, grep from subdomainscombined using the domain, output to associated dir, then run shuffledns.
		RunShufflednsCommand := "shuffledns -r ./wordlists/resolvers.txt -d " + domain + " -list " + outputPath + "subdomains.txt -o " + outputPath + "shuffledns.out -t 2000 -wt 100  -mcmd '-s 2000'"
		fmt.Println("Running shuffledns mode 1 - " + RunShufflednsCommand)
		exec.Command("bash", "-c", RunShufflednsCommand).Output()
		//if err != nil {
		//	fmt.Println("Error shuffdns mode 1")
		//}
		//fmt.Println(string(RunShufflednsOut))
		wg.Done()
	}
	if mode == "2" {
		// run shuffledns in mode 2: runs after dnsgen
		RunShufflednsCommand := "shuffledns -r ./wordlists/resolvers.txt  -d " + domain + " -list " + outputPath + "dnsgen.out -o " + outputPath + "subdomains-results-massdns.txt -t 2000 -wt 100 -mcmd '-s 2000'"
		fmt.Println("Running shuffledns mode 2 - " + RunShufflednsCommand)
		exec.Command("bash", "-c", RunShufflednsCommand).Output()
		//if err != nil {
		//	fmt.Println("Error massdns mode 2")
		//}
		//fmt.Println(string(RunShufflednsOut))
		wg.Done()
	}
}

func RunDNSGen(fleetName string, outputPath string, wg *sync.WaitGroup) {
	runDNSGenCommand := "dnsgen -f " + outputPath + "shuffledns.out > " + outputPath + "dnsgen.out"
	fmt.Println("Running dnsgen - " + runDNSGenCommand)
	exec.Command("bash", "-c", runDNSGenCommand).Output()
	//if err != nil {
	//	log.Fatal(err)
	//}
	//fmt.Println(string(runDNSGenOut))
	wg.Done()
}
