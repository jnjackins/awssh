package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sort"

	"regexp"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
)

func main() {
	log.SetPrefix("awssh: ")
	log.SetFlags(0)

	var (
		rflag = flag.Bool("r", false, "Refresh instance cache.")
		lflag = flag.Bool("l", false, "List instances only.")
	)
	flag.Parse()

	cachePath := os.TempDir() + "awssh.cache"
	f, err := os.Open(cachePath)
	if os.IsNotExist(err) {
		*rflag = true
	}
	if err != nil {
		log.Fatal(err)
	}

	var instances []string
	if *rflag {
		f, err = os.Create(cachePath)
		if err != nil {
			log.Fatal(err)
		}
		instances, err = updateCache(f)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		r := bufio.NewScanner(f)
		for r.Scan() {
			instances = append(instances, r.Text())
		}
		if err := r.Err(); err != nil {
			log.Fatal(err)
		}
	}

	patterns := make([]*regexp.Regexp, flag.NArg())
	for i := 0; i < flag.NArg(); i++ {
		pattern, err := regexp.Compile(flag.Arg(i))
		if err != nil {
			log.Fatal(err)
		}
		patterns[i] = pattern
	}

	var matches []string
outer:
	for _, instance := range instances {
		for _, pattern := range patterns {
			if !pattern.MatchString(instance) {
				continue outer
			}
		}
		matches = append(matches, instance)
	}

	if *lflag {
		for _, instance := range matches {
			fmt.Println(instance)
		}
		return
	}

	var host string
	if len(matches) > 1 {
		for i, instance := range matches {
			fmt.Printf("%d) %s\n", i, instance)
		}
		fmt.Println()
		fmt.Print("? ")

		var n int
		if _, err := fmt.Scanln(&n); err != nil {
			log.Fatal(err)
		}
		if n < 0 || n >= len(matches) {
			log.Fatalf("invalid host number %d", n)
		}
		host = matches[n]
	} else if len(matches) == 1 {
		host = matches[0]
	} else {
		log.Fatal("no matches")
	}

	cmd := exec.Command("ssh", "-tt", host)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}
}

func updateCache(f *os.File) ([]string, error) {
	instances, err := getInstances()
	if err != nil {
		return nil, err
	}

	if err := f.Truncate(0); err != nil {
		return nil, err
	}
	w := bufio.NewWriter(f)
	for _, instance := range instances {
		w.WriteString(instance + "\n")
	}
	w.Flush()
	f.Close()

	return instances, nil
}

func getInstances() ([]string, error) {
	ssn := session.New()
	*ssn.Config.Region = os.Getenv("AWS_DEFAULT_REGION")
	service := ec2.New(ssn)

	resp, err := service.DescribeInstances(nil)
	if err != nil {
		return nil, err
	}

	var instances []string
	for idx := range resp.Reservations {
		for _, inst := range resp.Reservations[idx].Instances {
			tags := make(map[string]string)
			for _, tag := range inst.Tags {
				tags[*tag.Key] = *tag.Value
			}
			instance := fmt.Sprintf("%s.%s.%s.%s.internal",
				tags["Name"], tags["Vpc"], tags["Subaccount"], tags["Owner"])
			instances = append(instances, instance)
		}
	}
	sort.Strings(instances)
	return instances, nil
}
