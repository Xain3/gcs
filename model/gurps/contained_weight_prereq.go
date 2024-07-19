// Copyright (c) 1998-2024 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

package gurps

import (
	"encoding/binary"
	"hash"

	"github.com/richardwilkes/gcs/v5/model/fxp"
	"github.com/richardwilkes/gcs/v5/model/gurps/enums/prereq"
	"github.com/richardwilkes/toolbox/i18n"
	"github.com/richardwilkes/toolbox/xio"
)

var _ Prereq = &ContainedWeightPrereq{}

// ContainedWeightPrereq holds a prerequisite for an equipment contained weight.
type ContainedWeightPrereq struct {
	Parent         *PrereqList    `json:"-"`
	Type           prereq.Type    `json:"type"`
	Has            bool           `json:"has"`
	WeightCriteria WeightCriteria `json:"qualifier,omitempty"`
}

// NewContainedWeightPrereq creates a new ContainedWeightPrereq.
func NewContainedWeightPrereq(entity *Entity) *ContainedWeightPrereq {
	return &ContainedWeightPrereq{
		Type: prereq.ContainedWeight,
		WeightCriteria: WeightCriteria{
			WeightCriteriaData: WeightCriteriaData{
				Compare:   AtMostNumber,
				Qualifier: fxp.WeightFromInteger(5, SheetSettingsFor(entity).DefaultWeightUnits),
			},
		},
		Has: true,
	}
}

// PrereqType implements Prereq.
func (c *ContainedWeightPrereq) PrereqType() prereq.Type {
	return c.Type
}

// ParentList implements Prereq.
func (c *ContainedWeightPrereq) ParentList() *PrereqList {
	return c.Parent
}

// Clone implements Prereq.
func (c *ContainedWeightPrereq) Clone(parent *PrereqList) Prereq {
	clone := *c
	clone.Parent = parent
	return &clone
}

// FillWithNameableKeys implements Prereq.
func (c *ContainedWeightPrereq) FillWithNameableKeys(_, _ map[string]string) {
}

// Satisfied implements Prereq.
func (c *ContainedWeightPrereq) Satisfied(entity *Entity, exclude any, tooltip *xio.ByteBuffer, prefix string, _ *bool) bool {
	satisfied := false
	if eqp, ok := exclude.(*Equipment); ok {
		if satisfied = !eqp.Container(); !satisfied {
			units := SheetSettingsFor(entity).DefaultWeightUnits
			weight := eqp.ExtendedWeight(false, units) - eqp.AdjustedWeight(false, units)
			satisfied = c.WeightCriteria.Matches(weight)
		}
	}
	if !c.Has {
		satisfied = !satisfied
	}
	if !satisfied && tooltip != nil {
		tooltip.WriteString(prefix)
		tooltip.WriteString(HasText(c.Has))
		tooltip.WriteString(i18n.Text(" a contained weight which "))
		tooltip.WriteString(c.WeightCriteria.String())
	}
	return satisfied
}

// Hash writes this object's contents into the hasher.
func (c *ContainedWeightPrereq) Hash(h hash.Hash) {
	if c == nil {
		return
	}
	_ = binary.Write(h, binary.LittleEndian, c.Type)
	_ = binary.Write(h, binary.LittleEndian, c.Has)
	c.WeightCriteria.Hash(h)
}
