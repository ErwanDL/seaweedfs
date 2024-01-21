package pub_balancer

import (
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/seaweedfs/seaweedfs/weed/glog"
	"github.com/seaweedfs/seaweedfs/weed/pb/mq_pb"
	"math/rand"
	"time"
)

func AllocateTopicPartitions(brokers cmap.ConcurrentMap[string, *BrokerStats], partitionCount int32) (assignments []*mq_pb.BrokerPartitionAssignment) {
	// divide the ring into partitions
	now := time.Now().UnixNano()
	rangeSize := MaxPartitionCount / partitionCount
	for i := int32(0); i < partitionCount; i++ {
		assignment := &mq_pb.BrokerPartitionAssignment{
			Partition: &mq_pb.Partition{
				RingSize:   MaxPartitionCount,
				RangeStart: int32(i * rangeSize),
				RangeStop:  int32((i + 1) * rangeSize),
				UnixTimeNs: now,
			},
		}
		if i == partitionCount-1 {
			assignment.Partition.RangeStop = MaxPartitionCount
		}
		assignments = append(assignments, assignment)
	}

	// pick the brokers
	pickedBrokers := pickBrokers(brokers, partitionCount)

	// assign the partitions to brokers
	for i, assignment := range assignments {
		assignment.LeaderBroker = pickedBrokers[i]
	}
	glog.V(0).Infof("allocate topic partitions %d: %v", len(assignments), assignments)
	return
}

// for now: randomly pick brokers
// TODO pick brokers based on the broker stats
func pickBrokers(brokers cmap.ConcurrentMap[string, *BrokerStats], count int32) []string {
	candidates := make([]string, 0, brokers.Count())
	for brokerStatsItem := range brokers.IterBuffered() {
		candidates = append(candidates, brokerStatsItem.Key)
	}
	pickedBrokers := make([]string, 0, count)
	for i := int32(0); i < count; i++ {
		p := rand.Int() % len(candidates)
		if p < 0 {
			p = -p
		}
		pickedBrokers = append(pickedBrokers, candidates[p])
	}
	return pickedBrokers
}

func EnsureAssignmentsToActiveBrokers(activeBrokers cmap.ConcurrentMap[string,*BrokerStats], assignments []*mq_pb.BrokerPartitionAssignment) (changedAssignments []*mq_pb.BrokerPartitionAssignment) {
	for _, assignment := range assignments {
		if assignment.LeaderBroker == "" {
			changedAssignments = append(changedAssignments, assignment)
			continue
		}
		if _, found := activeBrokers.Get(assignment.LeaderBroker); !found {
			changedAssignments = append(changedAssignments, assignment)
			continue
		}
	}

	// pick the brokers with the least number of partitions
	pickedBrokers := pickBrokers(activeBrokers, int32(len(changedAssignments)))
	for i, assignment := range changedAssignments {
		assignment.LeaderBroker = pickedBrokers[i]
	}
	return changedAssignments
}
