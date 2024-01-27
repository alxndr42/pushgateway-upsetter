package main

import (
	"flag"
	"log"
	"time"

	"upsetter/pushgateway"
	"upsetter/tracking"
)

const defaultRefresh = "20s"
const defaultURL = "http://localhost:9091"

func main() {
	log.SetFlags(0)

	refreshFlag := flag.String("refresh", defaultRefresh, "Refresh period")
	urlFlag := flag.String("url", defaultURL, "Pushgateway URL")
	flag.Parse()

	refreshPeriod, err := time.ParseDuration(*refreshFlag)
	if err != nil {
		log.Fatalf("Error parsing refresh period: %v", err)
	}

	client := pushgateway.NewPushgateway(*urlFlag)
	states := map[string]*tracking.GroupState{}

	for _ = range time.Tick(refreshPeriod) {
		groups, err := client.QueryMetrics()
		if err != nil {
			log.Printf("Error querying metrics: %v", err)
			continue
		}

		for _, group := range groups {
			if !group.LabelNamesMatch("job", "instance") {
				continue
			}

			key := group.Key()
			metrics := group.Metrics.Filter("up", "push_time_seconds", "push_failure_time_seconds")
			timestamp := metrics.MinTimestamp()

			state, ok := states[key]
			if !ok {
				states[key] = tracking.NewGroupState(timestamp)
				log.Printf("Group added: %v", key)
				continue
			}

			if state.Update(timestamp) {
				up := state.IsUp()
				if up {
					log.Printf("Group up: %v", key)
				} else {
					log.Printf("Group down: %v", key)
				}
				err := client.Upset(key, up)
				if err != nil {
					log.Printf("Error upsetting %s: %v", key, err)
				}
			}
		}
	}
}
