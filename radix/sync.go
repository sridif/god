package radix

import (
	"../common"
	"bytes"
)

const (
	subTreeError = "Illegal, only one level of sub trees supported"
)

type HashTree interface {
	Hash() []byte

	Finger(key []Nibble) *Print
	GetTimestamp(key []Nibble) (byteValue []byte, timestamp int64, present bool)
	PutTimestamp(key []Nibble, byteValue []byte, present bool, expected, timestamp int64) bool
	DelTimestamp(key []Nibble, expected int64) bool

	SubFinger(key, subKey []Nibble) (result *Print)
	SubGetTimestamp(key, subKey []Nibble) (byteValue []byte, timestamp int64, present bool)
	SubPutTimestamp(key, subKey []Nibble, byteValue []byte, present bool, subExpected, subTimestamp int64) bool
	SubDelTimestamp(key, subKey []Nibble, subExpected int64) bool
}

type Sync struct {
	source      HashTree
	destination HashTree
	from        []Nibble
	to          []Nibble
	destructive bool
	putCount    int
	delCount    int
}

func NewSync(source, destination HashTree) *Sync {
	return &Sync{
		source:      source,
		destination: destination,
	}
}

/*
Inclusive
*/
func (self *Sync) From(from []byte) *Sync {
	self.from = Rip(from)
	return self
}

/*
Exclusive
*/
func (self *Sync) To(to []byte) *Sync {
	self.to = Rip(to)
	return self
}
func (self *Sync) Destroy() *Sync {
	self.destructive = true
	return self
}
func (self *Sync) PutCount() int {
	return self.putCount
}
func (self *Sync) DelCount() int {
	return self.delCount
}
func (self *Sync) Run() *Sync {
	// If we have from and to, and they are equal, that means this sync is over an empty set... just ignore it
	if self.from != nil && self.to != nil && nComp(self.from, self.to) == 0 {
		return self
	}
	if self.destructive || bytes.Compare(self.source.Hash(), self.destination.Hash()) != 0 {
		self.synchronize(self.source.Finger(nil), self.destination.Finger(nil))
	}
	return self
}
func (self *Sync) potentiallyWithinLimits(key []Nibble) bool {
	if self.from == nil || self.to == nil {
		return true
	}
	cmpKey := toBytes(key)
	cmpFrom := toBytes(self.from)
	cmpTo := toBytes(self.to)
	m := len(cmpKey)
	if m > len(cmpFrom) {
		m = len(cmpFrom)
	}
	if m > len(cmpTo) {
		m = len(cmpTo)
	}
	return common.BetweenII(cmpKey[:m], cmpFrom[:m], cmpTo[:m])
}
func (self *Sync) withinLimits(key []Nibble) bool {
	if self.from == nil || self.to == nil {
		return true
	}
	return common.BetweenIE(toBytes(key), toBytes(self.from), toBytes(self.to))
}
func (self *Sync) synchronize(sourcePrint, destinationPrint *Print) {
	if sourcePrint.Exists {
		if !sourcePrint.Empty && self.withinLimits(sourcePrint.Key) {
			if sourcePrint.SubTree && (self.destructive || bytes.Compare(sourcePrint.TreeHash, destinationPrint.TreeHash) != 0) {
				subSync := NewSync(&subTreeWrapper{
					self.source,
					sourcePrint.Key,
				}, &subTreeWrapper{
					self.destination,
					sourcePrint.Key,
				})
				if self.destructive {
					subSync.Destroy()
				}
				subSync.Run()
				self.putCount += subSync.PutCount()
				self.delCount += subSync.DelCount()
			}
			if sourcePrint.Timestamp > 0 {
				if !sourcePrint.coveredBy(destinationPrint) {
					if value, timestamp, present := self.source.GetTimestamp(sourcePrint.Key); timestamp == sourcePrint.timestamp() {
						if self.destination.PutTimestamp(sourcePrint.Key, value, present, destinationPrint.timestamp(), sourcePrint.timestamp()) {
							self.putCount++
						}
					}
				}
				if self.destructive && !sourcePrint.Empty {
					if self.source.DelTimestamp(sourcePrint.Key, sourcePrint.timestamp()) {
						self.delCount++
					}
				}
			}
		}
		for index, subPrint := range sourcePrint.SubPrints {
			if subPrint.Exists && self.potentiallyWithinLimits(subPrint.Key) {
				if self.destructive || (!destinationPrint.Exists || !subPrint.equals(destinationPrint.SubPrints[index])) {
					self.synchronize(
						self.source.Finger(subPrint.Key),
						self.destination.Finger(subPrint.Key),
					)
				}
			}
		}
	}
}
