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
	"strings"

	"github.com/richardwilkes/gcs/v5/model/fxp"
	"github.com/richardwilkes/toolbox/i18n"
)

var skillBasedDefaultTypes = map[string]bool{
	SkillID: true,
	ParryID: true,
	BlockID: true,
}

// SkillDefault holds data for a Skill default.
type SkillDefault struct {
	DefaultType    string  `json:"type"`
	Name           string  `json:"name,omitempty"`
	Specialization string  `json:"specialization,omitempty"`
	Modifier       fxp.Int `json:"modifier,omitempty"`
	Level          fxp.Int `json:"level,omitempty"`
	AdjLevel       fxp.Int `json:"adjusted_level,omitempty"`
	Points         fxp.Int `json:"points,omitempty"`
}

// DefaultTypeIsSkillBased returns true if the SkillDefault type is Skill-based.
func DefaultTypeIsSkillBased(skillDefaultType string) bool {
	return skillBasedDefaultTypes[strings.ToLower(strings.TrimSpace(skillDefaultType))]
}

// CloneWithoutLevelOrPoints creates a copy, but without the level or points set.
func (s *SkillDefault) CloneWithoutLevelOrPoints() *SkillDefault {
	clone := *s
	clone.Level = 0
	clone.AdjLevel = 0
	clone.Points = 0
	return &clone
}

// Equivalent returns true if this can be considered equivalent to other.
func (s *SkillDefault) Equivalent(replacements map[string]string, other *SkillDefault) bool {
	return other != nil && s.DefaultType == other.DefaultType && s.Modifier == other.Modifier &&
		s.NameWithReplacements(replacements) == other.NameWithReplacements(replacements) &&
		s.SpecializationWithReplacements(replacements) == other.SpecializationWithReplacements(replacements)
}

// Type returns the type of the SkillDefault.
func (s *SkillDefault) Type() string {
	return s.DefaultType
}

// SetType sets the type of the SkillDefault.
func (s *SkillDefault) SetType(t string) {
	s.DefaultType = SanitizeID(t, true)
}

// FullName returns the full name of the skill to default from.
func (s *SkillDefault) FullName(entity *Entity, replacements map[string]string) string {
	if s.SkillBased() {
		var buffer strings.Builder
		buffer.WriteString(s.NameWithReplacements(replacements))
		if s.Specialization != "" {
			buffer.WriteString(" (")
			buffer.WriteString(s.SpecializationWithReplacements(replacements))
			buffer.WriteByte(')')
		}
		switch {
		case strings.EqualFold(DodgeID, s.DefaultType):
			buffer.WriteString(i18n.Text(" Dodge"))
		case strings.EqualFold(ParryID, s.DefaultType):
			buffer.WriteString(i18n.Text(" Parry"))
		case strings.EqualFold(BlockID, s.DefaultType):
			buffer.WriteString(i18n.Text(" Block"))
		}
		return buffer.String()
	}
	return ResolveAttributeName(entity, s.DefaultType)
}

// NameWithReplacements returns the name of the skill to default from with any nameable keys replaced.
func (s *SkillDefault) NameWithReplacements(replacements map[string]string) string {
	return ApplyNameables(s.Name, replacements)
}

// SpecializationWithReplacements returns the specialization of the skill to default from with any nameable keys
// replaced.
func (s *SkillDefault) SpecializationWithReplacements(replacements map[string]string) string {
	return ApplyNameables(s.Specialization, replacements)
}

// FillWithNameableKeys adds any nameable keys found in this SkillDefault to the provided map.
func (s *SkillDefault) FillWithNameableKeys(m, existing map[string]string) {
	ExtractNameables(s.Name, m, existing)
	ExtractNameables(s.Specialization, m, existing)
}

// ModifierAsString returns the modifier as a string suitable for appending.
func (s *SkillDefault) ModifierAsString() string {
	if s.Modifier != 0 {
		return s.Modifier.StringWithSign()
	}
	return ""
}

// SkillBased returns true if the Type() is Skill-based.
func (s *SkillDefault) SkillBased() bool {
	return skillBasedDefaultTypes[strings.ToLower(strings.TrimSpace(s.DefaultType))]
}

// SkillLevel returns the base skill level for this SkillDefault.
func (s *SkillDefault) SkillLevel(entity *Entity, replacements map[string]string, requirePoints bool, excludes map[string]bool, ruleOf20 bool) fxp.Int {
	switch s.Type() {
	case ParryID:
		best := s.best(entity, replacements, requirePoints, excludes)
		if best != fxp.Min {
			best = best.Div(fxp.Two).Trunc() + fxp.Three + entity.ParryBonus
		}
		return s.finalLevel(best)
	case BlockID:
		best := s.best(entity, replacements, requirePoints, excludes)
		if best != fxp.Min {
			best = best.Div(fxp.Two).Trunc() + fxp.Three + entity.BlockBonus
		}
		return s.finalLevel(best)
	case SkillID:
		return s.finalLevel(s.best(entity, replacements, requirePoints, excludes))
	default:
		return s.SkillLevelFast(entity, replacements, requirePoints, excludes, ruleOf20)
	}
}

func (s *SkillDefault) best(entity *Entity, replacements map[string]string, requirePoints bool, excludes map[string]bool) fxp.Int {
	best := fxp.Min
	for _, sk := range entity.SkillNamed(s.NameWithReplacements(replacements),
		s.SpecializationWithReplacements(replacements), requirePoints, excludes) {
		if best < sk.LevelData.Level {
			level := sk.CalculateLevel(excludes).Level
			if best < level {
				best = level
			}
		}
	}
	return best
}

// SkillLevelFast returns the base skill level for this SkillDefault.
func (s *SkillDefault) SkillLevelFast(entity *Entity, replacements map[string]string, requirePoints bool, excludes map[string]bool, ruleOf20 bool) fxp.Int {
	switch s.Type() {
	case DodgeID:
		level := entity.Dodge(entity.EncumbranceLevel(false))
		if ruleOf20 && level > 20 {
			level = 20
		}
		return s.finalLevel(fxp.From(level))
	case ParryID:
		best := s.bestFast(entity, replacements, requirePoints, excludes)
		if best != fxp.Min {
			best = best.Div(fxp.Two).Trunc() + fxp.Three + entity.ParryBonus
		}
		return s.finalLevel(best)
	case BlockID:
		best := s.bestFast(entity, replacements, requirePoints, excludes)
		if best != fxp.Min {
			best = best.Div(fxp.Two).Trunc() + fxp.Three + entity.BlockBonus
		}
		return s.finalLevel(best)
	case SkillID:
		return s.finalLevel(s.bestFast(entity, replacements, requirePoints, excludes))
	default:
		level := entity.ResolveAttributeCurrent(s.Type())
		if ruleOf20 {
			level = level.Min(fxp.Twenty)
		}
		if entity.SheetSettings.UseHalfStatDefaults {
			level = level.Div(fxp.Two).Trunc() + fxp.Five
		}
		return s.finalLevel(level)
	}
}

func (s *SkillDefault) bestFast(entity *Entity, replacements map[string]string, requirePoints bool, excludes map[string]bool) fxp.Int {
	best := fxp.Min
	for _, sk := range entity.SkillNamed(s.NameWithReplacements(replacements),
		s.SpecializationWithReplacements(replacements), requirePoints, excludes) {
		if best < sk.LevelData.Level {
			best = sk.LevelData.Level
		}
	}
	return best
}

func (s *SkillDefault) finalLevel(level fxp.Int) fxp.Int {
	if level != fxp.Min {
		level += s.Modifier
	}
	return level
}

// Hash writes this object's contents into the hasher. Note that this only hashes the data that is considered to be
// "source" data, i.e. not expected to be modified by the user after copying from a library.
func (s *SkillDefault) Hash(h hash.Hash) {
	_, _ = h.Write([]byte(s.DefaultType))
	_, _ = h.Write([]byte(s.Name))
	_, _ = h.Write([]byte(s.Specialization))
	_ = binary.Write(h, binary.LittleEndian, s.Modifier)
}
