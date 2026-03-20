package node

import (
	"math"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/Resinat/Resin/internal/model"
)

func loadAtomicString(ptr *atomic.Pointer[string]) string {
	value := ptr.Load()
	if value == nil {
		return ""
	}
	return *value
}

func storeAtomicString(ptr *atomic.Pointer[string], value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		ptr.Store(nil)
		return
	}
	copyValue := value
	ptr.Store(&copyValue)
}

func (e *NodeEntry) GetEgressNetworkType() model.EgressNetworkType {
	return model.NormalizeEgressNetworkType(loadAtomicString(&e.egressNetworkType))
}

func (e *NodeEntry) SetEgressNetworkType(value model.EgressNetworkType) {
	storeAtomicString(&e.egressNetworkType, string(model.NormalizeEgressNetworkType(string(value))))
}

func (e *NodeEntry) GetEgressASN() int64 {
	return e.egressASN.Load()
}

func (e *NodeEntry) SetEgressASN(value int64) {
	if value < 0 {
		value = 0
	}
	e.egressASN.Store(value)
}

func (e *NodeEntry) GetEgressASNName() string {
	return loadAtomicString(&e.egressASNName)
}

func (e *NodeEntry) SetEgressASNName(value string) {
	storeAtomicString(&e.egressASNName, value)
}

func (e *NodeEntry) GetEgressASNType() string {
	return loadAtomicString(&e.egressASNType)
}

func (e *NodeEntry) SetEgressASNType(value string) {
	storeAtomicString(&e.egressASNType, strings.ToUpper(strings.TrimSpace(value)))
}

func (e *NodeEntry) GetEgressProvider() string {
	return loadAtomicString(&e.egressProvider)
}

func (e *NodeEntry) SetEgressProvider(value string) {
	storeAtomicString(&e.egressProvider, value)
}

func (e *NodeEntry) GetEgressProfileSource() model.EgressProfileSource {
	return model.NormalizeEgressProfileSource(loadAtomicString(&e.egressProfileSource))
}

func (e *NodeEntry) SetEgressProfileSource(value model.EgressProfileSource) {
	storeAtomicString(&e.egressProfileSource, string(model.NormalizeEgressProfileSource(string(value))))
}

func (e *NodeEntry) GetQualityGrade() model.QualityGrade {
	return model.NormalizeQualityGrade(loadAtomicString(&e.qualityGrade))
}

func (e *NodeEntry) SetQualityGrade(value model.QualityGrade) {
	storeAtomicString(&e.qualityGrade, string(model.NormalizeQualityGrade(string(value))))
}

func (e *NodeEntry) HasProfile() bool {
	return e.LastEgressProfileUpdated.Load() > 0
}

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func qualityGradeForScore(score int) model.QualityGrade {
	switch {
	case score >= 85:
		return model.QualityGradeA
	case score >= 70:
		return model.QualityGradeB
	case score >= 55:
		return model.QualityGradeC
	default:
		return model.QualityGradeD
	}
}

func (e *NodeEntry) ReferenceLatencyMs(authorities []string) (float64, bool) {
	if len(authorities) == 0 && e != nil && e.LatencyTable != nil && e.LatencyTable.Size() > 0 {
		var sumMs float64
		var count int
		e.LatencyTable.Range(func(_ string, stats DomainLatencyStats) bool {
			sumMs += float64(stats.Ewma.Milliseconds())
			count++
			return true
		})
		if count > 0 {
			return sumMs / float64(count), true
		}
	}
	return AverageEWMAForDomainsMs(e, authorities)
}

func (e *NodeEntry) EgressStabilityScore() int {
	successes := e.EgressProbeSuccessCountTotal.Load()
	changes := e.EgressIPChangeCountTotal.Load()
	ratio := 1.0 - float64(changes)/float64(maxInt64(1, successes))
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	return clampInt(int(math.Round(ratio*20)), 0, 20)
}

func (e *NodeEntry) RefreshQuality(authorities []string) bool {
	score := 0

	switch e.GetEgressNetworkType() {
	case model.EgressNetworkTypeResidential:
		score += 45
	case model.EgressNetworkTypeMobile:
		score += 40
	case model.EgressNetworkTypeUnknown:
		score += 20
	case model.EgressNetworkTypeDatacenter:
		score += 0
	}

	if latencyMs, ok := e.ReferenceLatencyMs(authorities); ok {
		switch {
		case latencyMs <= 200:
			score += 25
		case latencyMs >= 2000:
			score += 0
		default:
			ratio := (2000 - latencyMs) / 1800
			score += clampInt(int(math.Round(ratio*25)), 0, 25)
		}
	}

	score += e.EgressStabilityScore()

	successCount := e.EgressProbeSuccessCountTotal.Load()
	failureCount := e.EgressProbeFailureCountTotal.Load()
	circuitOpens := e.CircuitOpenCountTotal.Load()
	totalProbes := maxInt64(1, successCount+failureCount)
	circuitRatio := 1.0 - float64(circuitOpens)/float64(totalProbes)
	if circuitRatio < 0 {
		circuitRatio = 0
	}
	if circuitRatio > 1 {
		circuitRatio = 1
	}
	score += clampInt(int(math.Round(circuitRatio*10)), 0, 10)
	if e.IsCircuitOpen() {
		score -= 5
	}

	score = clampInt(score, 0, 100)
	grade := qualityGradeForScore(score)

	changed := false
	if int(e.QualityScore.Swap(int32(score))) != score {
		changed = true
	}
	if e.GetQualityGrade() != grade {
		e.SetQualityGrade(grade)
		changed = true
	}
	return changed
}

func (e *NodeEntry) SetEgressProfile(profile NodeProfile) bool {
	changed := false

	nextType := model.NormalizeEgressNetworkType(string(profile.NetworkType))
	if e.GetEgressNetworkType() != nextType {
		e.SetEgressNetworkType(nextType)
		changed = true
	}

	if e.GetEgressASN() != profile.ASN {
		e.SetEgressASN(profile.ASN)
		changed = true
	}
	if e.GetEgressASNName() != profile.ASNName {
		e.SetEgressASNName(profile.ASNName)
		changed = true
	}
	if e.GetEgressASNType() != strings.ToUpper(strings.TrimSpace(profile.ASNType)) {
		e.SetEgressASNType(profile.ASNType)
		changed = true
	}
	if e.GetEgressProvider() != strings.TrimSpace(profile.Provider) {
		e.SetEgressProvider(profile.Provider)
		changed = true
	}

	nextSource := model.NormalizeEgressProfileSource(string(profile.Source))
	if e.GetEgressProfileSource() != nextSource {
		e.SetEgressProfileSource(nextSource)
		changed = true
	}
	if profile.ProfileUpdatedAtNs > 0 && e.LastEgressProfileUpdated.Swap(profile.ProfileUpdatedAtNs) != profile.ProfileUpdatedAtNs {
		changed = true
	}
	return changed
}

type NodeProfile struct {
	IP                 netip.Addr
	NetworkType        model.EgressNetworkType
	ASN                int64
	ASNName            string
	ASNType            string
	Provider           string
	Source             model.EgressProfileSource
	ProfileUpdatedAtNs int64
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
