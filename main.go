package main

import (
	"flag"
	"log"
	"slices"
	"time"

	"upsetter/pushgateway"
	"upsetter/tracking"
)

const defaultRefresh = "20s"
const defaultTTL = "24h"
const defaultURL = "http://localhost:9091"

func main() {
	log.SetFlags(0)

	refreshFlag := flag.String("refresh", defaultRefresh, "Refresh period")
	ttlFlag := flag.String("ttl", defaultTTL, "Group TTL")
	urlFlag := flag.String("url", defaultURL, "Pushgateway URL")
	flag.Parse()

	refreshPeriod, err := time.ParseDuration(*refreshFlag)
	if err != nil {
		log.Fatalf("Error parsing refresh period: %v", err)
	}
	groupTTL, err := time.ParseDuration(*ttlFlag)
	if err != nil {
		log.Fatalf("Error parsing group TTL: %v", err)
	}

	client := pushgateway.NewPushgateway(*urlFlag)
	states := map[string]*tracking.GroupState{}

	for _ = range time.Tick(refreshPeriod) {
		groups, err := client.QueryMetrics()
		if err != nil {
			log.Printf("Error querying metrics: %v", err)
			continue
		}

		receivedKeys := make([]string, 0, len(groups))
		expirationTime := time.Now().Add(-groupTTL)

		for _, group := range groups {
			if !group.LabelNamesMatch("job", "instance") {
				continue
			}

			key := group.Key()
			receivedKeys = append(receivedKeys, key)
			metrics := group.Metrics.Filter("up", "push_time_seconds", "push_failure_time_seconds")
			timestamp := metrics.MinTimestamp()

			state, ok := states[key]
			if !ok {
				states[key] = tracking.NewGroupState(timestamp)
				log.Printf("Group added: %v", key)
				continue
			}

			if group.Metrics.MaxTimestamp().Before(expirationTime) {
				delete(states, key)
				log.Printf("Group expired: %v", key)
				err := client.Delete(key)
				if err != nil {
					log.Printf("Error deleting %s: %v", key, err)
				}
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

		for key := range states {
			if !slices.Contains(receivedKeys, key) {
				delete(states, key)
				log.Printf("Group removed: %v", key)
			}
		}
	}
}
