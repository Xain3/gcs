// Copyright (c) 1998-2024 by Richard A. Wilkes. All rights reserved.
//
// This Source Code Form is subject to the terms of the Mozilla Public
// License, version 2.0. If a copy of the MPL was not distributed with
// this file, You can obtain one at http://mozilla.org/MPL/2.0/.
//
// This Source Code Form is "Incompatible With Secondary Licenses", as
// defined by the Mozilla Public License, version 2.0.

package ux

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/richardwilkes/gcs/v5/model/criteria"
	"github.com/richardwilkes/gcs/v5/model/fonts"
	"github.com/richardwilkes/gcs/v5/model/fxp"
	"github.com/richardwilkes/gcs/v5/model/gurps"
	"github.com/richardwilkes/gcs/v5/model/gurps/enums/picker"
	"github.com/richardwilkes/gcs/v5/svg"
	"github.com/richardwilkes/toolbox/errs"
	"github.com/richardwilkes/toolbox/i18n"
	"github.com/richardwilkes/toolbox/tid"
	"github.com/richardwilkes/toolbox/xio/fs"
	"github.com/richardwilkes/unison"
	"github.com/richardwilkes/unison/enums/align"
	"github.com/richardwilkes/unison/enums/behavior"
	"github.com/richardwilkes/unison/enums/check"
	"github.com/richardwilkes/unison/enums/paintstyle"
)

var (
	_ FileBackedDockable         = &Template{}
	_ unison.UndoManagerProvider = &Template{}
	_ ModifiableRoot             = &Template{}
	_ Rebuildable                = &Template{}
	_ unison.TabCloser           = &Template{}
)

// Template holds the view for a GURPS character template.
type Template struct {
	unison.Panel
	path              string
	targetMgr         *TargetMgr
	undoMgr           *unison.UndoManager
	toolbar           *unison.Panel
	scroll            *unison.ScrollPanel
	template          *gurps.Template
	crc               uint64
	content           *templateContent
	Traits            *PageList[*gurps.Trait]
	Skills            *PageList[*gurps.Skill]
	Spells            *PageList[*gurps.Spell]
	Equipment         *PageList[*gurps.Equipment]
	Notes             *PageList[*gurps.Note]
	dragReroutePanel  *unison.Panel
	scale             int
	needsSaveAsPrompt bool
}

// OpenTemplates returns the currently open templates.
func OpenTemplates(exclude *Template) []*Template {
	var templates []*Template
	for _, one := range AllDockables() {
		if template, ok := one.(*Template); ok && template != exclude {
			templates = append(templates, template)
		}
	}
	return templates
}

// NewTemplateFromFile loads a GURPS template file and creates a new unison.Dockable for it.
func NewTemplateFromFile(filePath string) (unison.Dockable, error) {
	template, err := gurps.NewTemplateFromFile(os.DirFS(filepath.Dir(filePath)), filepath.Base(filePath))
	if err != nil {
		return nil, err
	}
	t := NewTemplate(filePath, template)
	t.needsSaveAsPrompt = false
	return t, nil
}

// NewTemplate creates a new unison.Dockable for GURPS template files.
func NewTemplate(filePath string, template *gurps.Template) *Template {
	t := &Template{
		path:              filePath,
		undoMgr:           unison.NewUndoManager(200, func(err error) { errs.Log(err) }),
		scroll:            unison.NewScrollPanel(),
		template:          template,
		scale:             gurps.GlobalSettings().General.InitialSheetUIScale,
		crc:               template.CRC64(),
		needsSaveAsPrompt: true,
	}
	t.Self = t
	t.targetMgr = NewTargetMgr(t)
	t.SetLayout(&unison.FlexLayout{
		Columns: 1,
		HAlign:  align.Fill,
		VAlign:  align.Fill,
	})

	t.MouseDownCallback = func(_ unison.Point, _, _ int, _ unison.Modifiers) bool {
		t.RequestFocus()
		return false
	}
	t.DataDragOverCallback = func(_ unison.Point, data map[string]any) bool {
		t.dragReroutePanel = nil
		for _, key := range dropKeys {
			if _, ok := data[key]; ok {
				if t.dragReroutePanel = t.keyToPanel(key); t.dragReroutePanel != nil {
					t.dragReroutePanel.DataDragOverCallback(unison.Point{Y: 100000000}, data)
					return true
				}
				break
			}
		}
		return false
	}
	t.DataDragExitCallback = func() {
		if t.dragReroutePanel != nil {
			t.dragReroutePanel.DataDragExitCallback()
			t.dragReroutePanel = nil
		}
	}
	t.DataDragDropCallback = func(_ unison.Point, data map[string]any) {
		if t.dragReroutePanel != nil {
			t.dragReroutePanel.DataDragDropCallback(unison.Point{Y: 10000000}, data)
			t.dragReroutePanel = nil
		}
	}
	t.DrawOverCallback = func(gc *unison.Canvas, _ unison.Rect) {
		if t.dragReroutePanel != nil {
			r := t.RectFromRoot(t.dragReroutePanel.RectToRoot(t.dragReroutePanel.ContentRect(true)))
			paint := unison.ThemeWarning.Paint(gc, r, paintstyle.Fill)
			paint.SetColorFilter(unison.Alpha30Filter())
			gc.DrawRect(r, paint)
		}
	}

	t.scroll.SetContent(t.createContent(), behavior.Unmodified, behavior.Unmodified)
	t.scroll.SetLayoutData(&unison.FlexLayoutData{
		HAlign: align.Fill,
		VAlign: align.Fill,
		HGrab:  true,
		VGrab:  true,
	})
	t.createToolbar()
	t.AddChild(t.scroll)

	t.InstallCmdHandlers(SaveItemID, func(_ any) bool { return t.Modified() }, func(_ any) { t.save(false) })
	t.InstallCmdHandlers(SaveAsItemID, unison.AlwaysEnabled, func(_ any) { t.save(true) })
	t.installNewItemCmdHandlers(NewTraitItemID, NewTraitContainerItemID, t.Traits)
	t.installNewItemCmdHandlers(NewSkillItemID, NewSkillContainerItemID, t.Skills)
	t.installNewItemCmdHandlers(NewTechniqueItemID, -1, t.Skills)
	t.installNewItemCmdHandlers(NewSpellItemID, NewSpellContainerItemID, t.Spells)
	t.installNewItemCmdHandlers(NewRitualMagicSpellItemID, -1, t.Spells)
	t.installNewItemCmdHandlers(NewCarriedEquipmentItemID,
		NewCarriedEquipmentContainerItemID, t.Equipment)
	t.installNewItemCmdHandlers(NewNoteItemID, NewNoteContainerItemID, t.Notes)
	t.InstallCmdHandlers(AddNaturalAttacksItemID, unison.AlwaysEnabled, func(_ any) {
		InsertItems[*gurps.Trait](t, t.Traits.Table, t.template.TraitList, t.template.SetTraitList,
			func(_ *unison.Table[*Node[*gurps.Trait]]) []*Node[*gurps.Trait] {
				return t.Traits.provider.RootRows()
			}, gurps.NewNaturalAttacks(nil, nil))
	})
	t.InstallCmdHandlers(ApplyTemplateItemID, t.canApplyTemplate, t.applyTemplate)
	t.InstallCmdHandlers(NewSheetFromTemplateItemID, unison.AlwaysEnabled, t.newSheetFromTemplate)

	t.template.EnsureAttachments()
	t.template.SourceMatcher().PrepareHashes(t.template)
	return t
}

func (t *Template) createToolbar() {
	t.toolbar = unison.NewPanel()
	t.AddChild(t.toolbar)
	t.toolbar.SetBorder(unison.NewCompoundBorder(unison.NewLineBorder(unison.ThemeSurfaceEdge, 0, unison.Insets{Bottom: 1},
		false), unison.NewEmptyBorder(unison.StdInsets())))
	t.toolbar.SetLayoutData(&unison.FlexLayoutData{
		HAlign: align.Fill,
		HGrab:  true,
	})
	t.toolbar.AddChild(NewDefaultInfoPop())

	helpButton := unison.NewSVGButton(svg.Help)
	helpButton.Tooltip = newWrappedTooltip(i18n.Text("Help"))
	helpButton.ClickCallback = func() { HandleLink(nil, "md:Help/Interface/Character Template") }
	t.toolbar.AddChild(helpButton)

	t.toolbar.AddChild(
		NewScaleField(
			gurps.InitialUIScaleMin,
			gurps.InitialUIScaleMax,
			func() int { return gurps.GlobalSettings().General.InitialSheetUIScale },
			func() int { return t.scale },
			func(scale int) { t.scale = scale },
			nil,
			false,
			t.scroll,
		),
	)

	addUserButton := unison.NewSVGButton(svg.Stamper)
	addUserButton.Tooltip = newWrappedTooltip(applyTemplateAction.Title)
	addUserButton.ClickCallback = func() {
		if CanApplyTemplate() {
			t.applyTemplate(nil)
		}
	}
	t.toolbar.AddChild(addUserButton)

	syncSourceButton := unison.NewSVGButton(svg.DownToBracket)
	syncSourceButton.Tooltip = newWrappedTooltip(i18n.Text("Sync with all sources in this sheet"))
	syncSourceButton.ClickCallback = func() { t.syncWithAllSources() }
	t.toolbar.AddChild(syncSourceButton)

	installSearchTracker(t.toolbar, func() {
		t.Traits.Table.ClearSelection()
		t.Skills.Table.ClearSelection()
		t.Spells.Table.ClearSelection()
		t.Equipment.Table.ClearSelection()
		t.Notes.Table.ClearSelection()
	}, func(refList *[]*searchRef, text string, namesOnly bool) {
		searchSheetTable(refList, text, namesOnly, t.Traits)
		searchSheetTable(refList, text, namesOnly, t.Skills)
		searchSheetTable(refList, text, namesOnly, t.Spells)
		searchSheetTable(refList, text, namesOnly, t.Equipment)
		searchSheetTable(refList, text, namesOnly, t.Notes)
	})

	t.toolbar.SetLayout(&unison.FlexLayout{
		Columns:  len(t.toolbar.Children()),
		HSpacing: unison.StdHSpacing,
	})
}

func (t *Template) keyToPanel(key string) *unison.Panel {
	var p unison.Paneler
	switch key {
	case equipmentDragKey:
		p = t.Equipment.Table
	case gurps.SkillID:
		p = t.Skills.Table
	case gurps.SpellID:
		p = t.Spells.Table
	case traitDragKey:
		p = t.Traits.Table
	case noteDragKey:
		p = t.Notes.Table
	default:
		return nil
	}
	return p.AsPanel()
}

// CanApplyTemplate returns true if a template can be applied.
func CanApplyTemplate() bool {
	return len(OpenSheets(nil)) > 0
}

func (t *Template) canApplyTemplate(_ any) bool {
	return CanApplyTemplate()
}

// NewSheetFromTemplate loads the specified template file and creates a new character sheet from it.
func NewSheetFromTemplate(filePath string) {
	t, err := NewTemplateFromFile(filePath)
	if err != nil {
		unison.ErrorDialogWithError(i18n.Text("Unable to load template"), err)
		return
	}
	if t, ok := t.(*Template); ok {
		t.newSheetFromTemplate(nil)
	}
}

func (t *Template) newSheetFromTemplate(_ any) {
	e := gurps.NewEntity()
	sheet := NewSheet(e.Profile.Name+gurps.SheetExt, e)
	DisplayNewDockable(sheet)
	if t.applyTemplateToSheet(sheet, true) {
		sheet.undoMgr.Clear()
		sheet.crc = 0
	}
	sheet.SetBackingFilePath(e.Profile.Name + gurps.SheetExt)
}

// ApplyTemplate loads the specified template file and applies it to a sheet.
func ApplyTemplate(filePath string) {
	t, err := NewTemplateFromFile(filePath)
	if err != nil {
		unison.ErrorDialogWithError(i18n.Text("Unable to load template"), err)
		return
	}
	if CanApplyTemplate() {
		if t, ok := t.(*Template); ok {
			t.applyTemplate(nil)
		}
	}
}

func (t *Template) applyTemplate(suppressRandomizePromptAsBool any) {
	suppressRandomizePrompt, _ := suppressRandomizePromptAsBool.(bool) //nolint:errcheck // The default of false on failure is acceptable
	for _, sheet := range PromptForDestination(OpenSheets(nil)) {
		t.applyTemplateToSheet(sheet, suppressRandomizePrompt)
	}
}

func (t *Template) applyTemplateToSheet(sheet *Sheet, suppressRandomizePrompt bool) bool {
	var undo *unison.UndoEdit[*ApplyTemplateUndoEditData]
	mgr := unison.UndoManagerFor(sheet)
	if mgr != nil {
		if beforeData, err := NewApplyTemplateUndoEditData(sheet); err != nil {
			errs.Log(err)
			mgr = nil
		} else {
			undo = &unison.UndoEdit[*ApplyTemplateUndoEditData]{
				ID:         unison.NextUndoID(),
				EditName:   i18n.Text("Apply Template"),
				UndoFunc:   func(e *unison.UndoEdit[*ApplyTemplateUndoEditData]) { e.BeforeData.Apply() },
				RedoFunc:   func(e *unison.UndoEdit[*ApplyTemplateUndoEditData]) { e.AfterData.Apply() },
				AbsorbFunc: func(_ *unison.UndoEdit[*ApplyTemplateUndoEditData], _ unison.Undoable) bool { return false },
				BeforeData: beforeData,
			}
		}
	}
	e := sheet.Entity()
	templateAncestries := gurps.ActiveAncestries(ExtractNodeDataFromList(t.Traits.Table.RootRows()))
	if len(templateAncestries) != 0 {
		entityAncestries := gurps.ActiveAncestries(e.Traits)
		if len(entityAncestries) != 0 {
			if unison.YesNoDialog(fmt.Sprintf(i18n.Text(`The template contains an Ancestry (%s).
Disable your character's existing Ancestry (%s)?`),
				templateAncestries[0].Name, entityAncestries[0].Name), "") == unison.ModalResponseOK {
				for _, one := range gurps.ActiveAncestryTraits(e.Traits) {
					one.Disabled = true
				}
			}
		}
	}
	traits := cloneRows(sheet.Traits.Table, t.Traits.Table.RootRows())
	skills := cloneRows(sheet.Skills.Table, t.Skills.Table.RootRows())
	spells := cloneRows(sheet.Spells.Table, t.Spells.Table.RootRows())
	equipment := cloneRows(sheet.CarriedEquipment.Table, t.Equipment.Table.RootRows())
	notes := cloneRows(sheet.Notes.Table, t.Notes.Table.RootRows())
	var abort bool
	if traits, abort = processPickerRows(traits); abort {
		return false
	}
	if skills, abort = processPickerRows(skills); abort {
		return false
	}
	if spells, abort = processPickerRows(spells); abort {
		return false
	}
	appendRows(sheet.Traits.Table, traits)
	appendRows(sheet.Skills.Table, skills)
	appendRows(sheet.Spells.Table, spells)
	appendRows(sheet.CarriedEquipment.Table, equipment)
	appendRows(sheet.Notes.Table, notes)
	sheet.Rebuild(true)
	ProcessModifiersForSelection(sheet.Traits.Table)
	ProcessModifiersForSelection(sheet.Skills.Table)
	ProcessModifiersForSelection(sheet.Spells.Table)
	ProcessModifiersForSelection(sheet.CarriedEquipment.Table)
	ProcessModifiersForSelection(sheet.Notes.Table)
	ProcessNameablesForSelection(sheet.Traits.Table)
	ProcessNameablesForSelection(sheet.Skills.Table)
	ProcessNameablesForSelection(sheet.Spells.Table)
	ProcessNameablesForSelection(sheet.CarriedEquipment.Table)
	ProcessNameablesForSelection(sheet.Notes.Table)
	if len(templateAncestries) != 0 && gurps.GlobalSettings().General.AutoFillProfile {
		randomize := true
		if !suppressRandomizePrompt {
			randomize = unison.YesNoDialog(i18n.Text("Would you like to apply the initial randomization again?"), "") == unison.ModalResponseOK
		}
		if randomize {
			e.Profile.ApplyRandomizers(e)
			updateRandomizedProfileFieldsWithoutUndo(sheet)
			sheet.Rebuild(true)
		}
	}
	if mgr != nil && undo != nil {
		var err error
		if undo.AfterData, err = NewApplyTemplateUndoEditData(sheet); err != nil {
			errs.Log(err)
		} else {
			mgr.Add(undo)
		}
	}
	sheet.Window().ToFront()
	sheet.RequestFocus()
	return true
}

func updateRandomizedProfileFieldsWithoutUndo(sheet *Sheet) {
	e := sheet.Entity()
	updateStringField(sheet, identityPanelNameFieldRefKey, e.Profile.Name)
	updateStringField(sheet, descriptionPanelAgeFieldRefKey, e.Profile.Age)
	updateStringField(sheet, descriptionPanelBirthdayFieldRefKey, e.Profile.Birthday)
	updateStringField(sheet, descriptionPanelEyesFieldRefKey, e.Profile.Eyes)
	updateStringField(sheet, descriptionPanelHairFieldRefKey, e.Profile.Hair)
	updateStringField(sheet, descriptionPanelSkinFieldRefKey, e.Profile.Skin)
	updateStringField(sheet, descriptionPanelHandednessFieldRefKey, e.Profile.Handedness)
	updateStringField(sheet, descriptionPanelGenderFieldRefKey, e.Profile.Gender)
	updateLengthField(sheet, descriptionPanelHeightFieldRefKey, e.Profile.Height)
	updateWeightField(sheet, descriptionPanelWeightFieldRefKey, e.Profile.Weight)
}

func updateStringField(sheet *Sheet, refKey, value string) {
	if panel := sheet.targetMgr.Find(refKey); panel != nil {
		if f, ok := panel.Self.(*StringField); ok {
			saved := sheet.undoMgr
			sheet.undoMgr = nil
			f.SetText(value)
			sheet.undoMgr = saved
		}
	}
}

func updateLengthField(sheet *Sheet, refKey string, value fxp.Length) {
	if panel := sheet.targetMgr.Find(refKey); panel != nil {
		if f, ok := panel.Self.(*LengthField); ok {
			saved := sheet.undoMgr
			sheet.undoMgr = nil
			f.SetText(value.String())
			sheet.undoMgr = saved
		}
	}
}

func updateWeightField(sheet *Sheet, refKey string, value fxp.Weight) {
	if panel := sheet.targetMgr.Find(refKey); panel != nil {
		if f, ok := panel.Self.(*WeightField); ok {
			saved := sheet.undoMgr
			sheet.undoMgr = nil
			f.SetText(value.String())
			sheet.undoMgr = saved
		}
	}
}

func cloneRows[T gurps.NodeTypes](table *unison.Table[*Node[T]], rows []*Node[T]) []*Node[T] {
	rows = slices.Clone(rows)
	for j, row := range rows {
		rows[j] = row.CloneForTarget(table, nil)
	}
	return rows
}

func appendRows[T gurps.NodeTypes](table *unison.Table[*Node[T]], rows []*Node[T]) {
	table.SetRootRows(append(slices.Clone(table.RootRows()), rows...))
	selMap := make(map[tid.TID]bool, len(rows))
	for _, row := range rows {
		selMap[row.ID()] = true
	}
	table.SetSelectionMap(selMap)
	if provider, ok := table.ClientData()[TableProviderClientKey]; ok {
		var tableProvider TableProvider[T]
		if tableProvider, ok = provider.(TableProvider[T]); ok {
			tableProvider.ProcessDropData(nil, table)
		}
	}
}

func processPickerRows[T gurps.NodeTypes](rows []*Node[T]) (revised []*Node[T], abort bool) {
	for _, one := range ExtractNodeDataFromList(rows) {
		result, cancel := processPickerRow(one)
		if cancel {
			return nil, true
		}
		for _, replacement := range result {
			revised = append(revised, NewNodeLike(rows[0], replacement))
		}
	}
	return revised, false
}

func processPickerRow[T gurps.NodeTypes](row T) (revised []T, abort bool) {
	n := gurps.AsNode[T](row)
	if !n.Container() {
		return []T{row}, false
	}
	children := n.NodeChildren()
	tpp, ok := n.(gurps.TemplatePickerProvider)
	if !ok || tpp.TemplatePickerData().ShouldOmit() {
		rowChildren := make([]T, 0, len(children))
		for _, child := range children {
			var result []T
			result, abort = processPickerRow(child)
			if abort {
				return nil, true
			}
			rowChildren = append(rowChildren, result...)
		}
		n.SetChildren(rowChildren)
		SetParents(rowChildren, row)
		return []T{row}, false
	}
	tp := tpp.TemplatePickerData()

	list := unison.NewPanel()
	list.SetBorder(unison.NewEmptyBorder(unison.NewUniformInsets(unison.StdHSpacing)))
	list.SetLayout(&unison.FlexLayout{
		Columns:  1,
		HSpacing: unison.StdHSpacing,
		VSpacing: unison.StdVSpacing,
	})

	boxes := make([]*unison.CheckBox, 0, len(children))
	var dialog *unison.Dialog
	callback := func() {
		var total fxp.Int
		for i, box := range boxes {
			if box.State == check.On {
				switch tp.Type {
				case picker.NotApplicable:
				case picker.Count:
					total += fxp.One
				case picker.Points:
					total += rawPoints(children[i])
				}
			}
		}
		dialog.Button(unison.ModalResponseOK).SetEnabled(tp.Qualifier.Matches(total))
	}
	for _, child := range children {
		checkBox := unison.NewCheckBox()
		title := child.String()
		if tp.Type == picker.Points {
			points := rawPoints(child)
			pointsLabel := i18n.Text("points")
			if points == fxp.One {
				pointsLabel = i18n.Text("point")
			}
			title += fmt.Sprintf(" [%s %s]", points.Comma(), pointsLabel)
		}
		checkBox.SetTitle(title)
		checkBox.ClickCallback = callback
		boxes = append(boxes, checkBox)
		list.AddChild(checkBox)
	}

	scroll := unison.NewScrollPanel()
	scroll.SetBorder(unison.NewLineBorder(unison.ThemeSurfaceEdge, 0, unison.NewUniformInsets(1), false))
	scroll.SetContent(list, behavior.Fill, behavior.Fill)
	scroll.BackgroundInk = unison.ThemeSurface
	scroll.SetLayoutData(&unison.FlexLayoutData{
		HAlign: align.Fill,
		VAlign: align.Fill,
		HGrab:  true,
		VGrab:  true,
	})

	panel := unison.NewPanel()
	panel.SetLayout(&unison.FlexLayout{
		Columns:  1,
		HSpacing: unison.StdHSpacing,
		VSpacing: unison.StdVSpacing,
		HAlign:   align.Fill,
		VAlign:   align.Fill,
	})
	label := unison.NewLabel()
	label.SetTitle(row.String())
	panel.AddChild(label)
	if notesCapable, hasNotes := any(row).(interface{ Notes() string }); hasNotes {
		if notes := notesCapable.Notes(); notes != "" {
			label = unison.NewLabel()
			label.Font = fonts.FieldSecondary
			label.SetTitle(notes)
			panel.AddChild(label)
		}
	}
	label = unison.NewLabel()
	label.SetTitle(tp.Description())
	label.SetBorder(unison.NewEmptyBorder(unison.Insets{Top: unison.StdVSpacing * 2}))
	panel.AddChild(label)
	panel.AddChild(scroll)

	var err error
	dialog, err = unison.NewDialog(unison.DefaultDialogTheme.QuestionIcon,
		unison.DefaultDialogTheme.QuestionIconInk, panel,
		[]*unison.DialogButtonInfo{unison.NewCancelButtonInfo(), unison.NewOKButtonInfo()})
	if err != nil {
		errs.Log(err)
		return nil, true
	}
	callback()
	if dialog.RunModal() == unison.ModalResponseCancel {
		return nil, true
	}

	rowChildren := make([]T, 0, len(children))
	for i, box := range boxes {
		if box.State == check.On {
			var result []T
			result, abort = processPickerRow(children[i])
			if abort {
				return nil, true
			}
			rowChildren = append(rowChildren, result...)
		}
	}
	SetParents(rowChildren, n.Parent())
	return rowChildren, false
}

func rawPoints(child any) fxp.Int {
	switch nc := child.(type) {
	case *gurps.Skill:
		if nc.Container() && nc.TemplatePicker != nil && nc.TemplatePicker.Type == picker.Points &&
			nc.TemplatePicker.Qualifier.Compare == criteria.EqualsNumber {
			return nc.TemplatePicker.Qualifier.Qualifier
		}
		return nc.RawPoints()
	case *gurps.Spell:
		if nc.Container() && nc.TemplatePicker != nil && nc.TemplatePicker.Type == picker.Points &&
			nc.TemplatePicker.Qualifier.Compare == criteria.EqualsNumber {
			return nc.TemplatePicker.Qualifier.Qualifier
		}
		return nc.RawPoints()
	case *gurps.Trait:
		if nc.Container() && nc.TemplatePicker != nil && nc.TemplatePicker.Type == picker.Points &&
			nc.TemplatePicker.Qualifier.Compare == criteria.EqualsNumber {
			return nc.TemplatePicker.Qualifier.Qualifier
		}
		return nc.AdjustedPoints()
	default:
		return 0
	}
}

func (t *Template) installNewItemCmdHandlers(itemID, containerID int, creator itemCreator) {
	variant := NoItemVariant
	if containerID == -1 {
		variant = AlternateItemVariant
	} else {
		t.InstallCmdHandlers(containerID, unison.AlwaysEnabled,
			func(_ any) { creator.CreateItem(t, ContainerItemVariant) })
	}
	t.InstallCmdHandlers(itemID, unison.AlwaysEnabled, func(_ any) { creator.CreateItem(t, variant) })
}

// Entity implements gurps.EntityProvider
func (t *Template) Entity() *gurps.Entity {
	return nil
}

// DockableKind implements widget.DockableKind
func (t *Template) DockableKind() string {
	return TemplateDockableKind
}

// UndoManager implements undo.Provider
func (t *Template) UndoManager() *unison.UndoManager {
	return t.undoMgr
}

// TitleIcon implements workspace.FileBackedDockable
func (t *Template) TitleIcon(suggestedSize unison.Size) unison.Drawable {
	return &unison.DrawableSVG{
		SVG:  gurps.FileInfoFor(t.path).SVG,
		Size: suggestedSize,
	}
}

// Title implements workspace.FileBackedDockable
func (t *Template) Title() string {
	return fs.BaseName(t.path)
}

func (t *Template) String() string {
	return t.Title()
}

// Tooltip implements workspace.FileBackedDockable
func (t *Template) Tooltip() string {
	return t.path
}

// BackingFilePath implements workspace.FileBackedDockable
func (t *Template) BackingFilePath() string {
	return t.path
}

// SetBackingFilePath implements workspace.FileBackedDockable
func (t *Template) SetBackingFilePath(p string) {
	t.path = p
	UpdateTitleForDockable(t)
}

// Modified implements workspace.FileBackedDockable
func (t *Template) Modified() bool {
	return t.crc != t.template.CRC64()
}

// MarkModified implements widget.ModifiableRoot.
func (t *Template) MarkModified(_ unison.Paneler) {
	UpdateTitleForDockable(t)
}

// MayAttemptClose implements unison.TabCloser
func (t *Template) MayAttemptClose() bool {
	return MayAttemptCloseOfGroup(t)
}

// AttemptClose implements unison.TabCloser
func (t *Template) AttemptClose() bool {
	if !CloseGroup(t) {
		return false
	}
	if t.Modified() {
		switch unison.YesNoCancelDialog(fmt.Sprintf(i18n.Text("Save changes made to\n%s?"), t.Title()), "") {
		case unison.ModalResponseDiscard:
		case unison.ModalResponseOK:
			if !t.save(false) {
				return false
			}
		case unison.ModalResponseCancel:
			return false
		}
	}
	return AttemptCloseForDockable(t)
}

func (t *Template) createContent() unison.Paneler {
	t.content = newTemplateContent()
	t.createLists()
	return t.content
}

func (t *Template) save(forceSaveAs bool) bool {
	success := false
	if forceSaveAs || t.needsSaveAsPrompt {
		success = SaveDockableAs(t, gurps.TemplatesExt, t.template.Save, func(path string) {
			t.crc = t.template.CRC64()
			t.path = path
		})
	} else {
		success = SaveDockable(t, t.template.Save, func() { t.crc = t.template.CRC64() })
	}
	if success {
		t.needsSaveAsPrompt = false
	}
	return success
}

func (t *Template) createLists() {
	h, v := t.scroll.Position()
	var refocusOnKey string
	var refocusOn unison.Paneler
	if wnd := t.Window(); wnd != nil {
		if focus := wnd.Focus(); focus != nil {
			// For page lists, the focus will be the table, so we need to look up a level
			if focus = focus.Parent(); focus != nil {
				switch focus.Self {
				case t.Traits:
					refocusOnKey = gurps.BlockLayoutTraitsKey
				case t.Skills:
					refocusOnKey = gurps.BlockLayoutSkillsKey
				case t.Spells:
					refocusOnKey = gurps.BlockLayoutSpellsKey
				case t.Equipment:
					refocusOnKey = gurps.BlockLayoutEquipmentKey
				case t.Notes:
					refocusOnKey = gurps.BlockLayoutNotesKey
				}
			}
		}
	}
	t.content.RemoveAllChildren()
	for _, col := range gurps.GlobalSettings().Sheet.BlockLayout.ByRow() {
		rowPanel := unison.NewPanel()
		for _, c := range col {
			switch c {
			case gurps.BlockLayoutTraitsKey:
				if t.Traits == nil {
					t.Traits = NewTraitsPageList(t, t.template)
				} else {
					t.Traits.Sync()
				}
				rowPanel.AddChild(t.Traits)
				if c == refocusOnKey {
					refocusOn = t.Traits.Table
				}
			case gurps.BlockLayoutSkillsKey:
				if t.Skills == nil {
					t.Skills = NewSkillsPageList(t, t.template)
				} else {
					t.Skills.Sync()
				}
				rowPanel.AddChild(t.Skills)
				if c == refocusOnKey {
					refocusOn = t.Skills.Table
				}
			case gurps.BlockLayoutSpellsKey:
				if t.Spells == nil {
					t.Spells = NewSpellsPageList(t, t.template)
				} else {
					t.Spells.Sync()
				}
				rowPanel.AddChild(t.Spells)
				if c == refocusOnKey {
					refocusOn = t.Spells.Table
				}
			case gurps.BlockLayoutEquipmentKey:
				if t.Equipment == nil {
					t.Equipment = NewCarriedEquipmentPageList(t, t.template)
				} else {
					t.Equipment.Sync()
				}
				rowPanel.AddChild(t.Equipment)
				if c == refocusOnKey {
					refocusOn = t.Equipment.Table
				}
			case gurps.BlockLayoutNotesKey:
				if t.Notes == nil {
					t.Notes = NewNotesPageList(t, t.template)
				} else {
					t.Notes.Sync()
				}
				rowPanel.AddChild(t.Notes)
				if c == refocusOnKey {
					refocusOn = t.Notes.Table
				}
			}
		}
		if len(rowPanel.Children()) != 0 {
			rowPanel.SetLayout(&unison.FlexLayout{
				Columns:      len(rowPanel.Children()),
				HSpacing:     1,
				HAlign:       align.Fill,
				EqualColumns: true,
			})
			rowPanel.SetLayoutData(&unison.FlexLayoutData{
				HAlign: align.Fill,
				HGrab:  true,
			})
			t.content.AddChild(rowPanel)
		}
	}
	t.content.ApplyPreferredSize()
	if refocusOn != nil {
		refocusOn.AsPanel().RequestFocus()
	}
	t.scroll.SetPosition(h, v)
}

// SheetSettingsUpdated implements gurps.SheetSettingsResponder.
func (t *Template) SheetSettingsUpdated(e *gurps.Entity, blockLayout bool) {
	if e == nil {
		t.Rebuild(blockLayout)
	}
}

// Rebuild implements widget.Rebuildable.
func (t *Template) Rebuild(full bool) {
	t.template.EnsureAttachments()
	t.template.SourceMatcher().PrepareHashes(t.template)
	h, v := t.scroll.Position()
	focusRefKey := t.targetMgr.CurrentFocusRef()
	if full {
		traitsSelMap := t.Traits.RecordSelection()
		skillsSelMap := t.Skills.RecordSelection()
		spellsSelMap := t.Spells.RecordSelection()
		equipmentSelMap := t.Equipment.RecordSelection()
		notesSelMap := t.Notes.RecordSelection()
		defer func() {
			t.Traits.ApplySelection(traitsSelMap)
			t.Skills.ApplySelection(skillsSelMap)
			t.Spells.ApplySelection(spellsSelMap)
			t.Equipment.ApplySelection(equipmentSelMap)
			t.Notes.ApplySelection(notesSelMap)
		}()
		t.createLists()
	}
	DeepSync(t)
	UpdateTitleForDockable(t)
	t.targetMgr.ReacquireFocus(focusRefKey, t.toolbar, t.scroll.Content())
	t.scroll.SetPosition(h, v)
}

type templateTablesUndoData struct {
	traits    *TableUndoEditData[*gurps.Trait]
	skills    *TableUndoEditData[*gurps.Skill]
	spells    *TableUndoEditData[*gurps.Spell]
	equipment *TableUndoEditData[*gurps.Equipment]
	notes     *TableUndoEditData[*gurps.Note]
}

func newTemplateTablesUndoData(t *Template) *templateTablesUndoData {
	return &templateTablesUndoData{
		traits:    NewTableUndoEditData(t.Traits.Table),
		skills:    NewTableUndoEditData(t.Skills.Table),
		spells:    NewTableUndoEditData(t.Spells.Table),
		equipment: NewTableUndoEditData(t.Equipment.Table),
		notes:     NewTableUndoEditData(t.Notes.Table),
	}
}

func (t *templateTablesUndoData) Apply() {
	t.traits.Apply()
	t.skills.Apply()
	t.spells.Apply()
	t.equipment.Apply()
	t.notes.Apply()
}

func (t *Template) syncWithAllSources() {
	var undo *unison.UndoEdit[*templateTablesUndoData]
	mgr := unison.UndoManagerFor(t)
	if mgr != nil {
		undo = &unison.UndoEdit[*templateTablesUndoData]{
			ID:         unison.NextUndoID(),
			EditName:   syncWithSourceAction.Title,
			UndoFunc:   func(e *unison.UndoEdit[*templateTablesUndoData]) { e.BeforeData.Apply() },
			RedoFunc:   func(e *unison.UndoEdit[*templateTablesUndoData]) { e.AfterData.Apply() },
			AbsorbFunc: func(_ *unison.UndoEdit[*templateTablesUndoData], _ unison.Undoable) bool { return false },
			BeforeData: newTemplateTablesUndoData(t),
		}
	}
	t.template.SyncWithLibrarySources()
	t.Traits.Table.SyncToModel()
	t.Skills.Table.SyncToModel()
	t.Spells.Table.SyncToModel()
	t.Equipment.Table.SyncToModel()
	t.Notes.Table.SyncToModel()
	if mgr != nil && undo != nil {
		undo.AfterData = newTemplateTablesUndoData(t)
		mgr.Add(undo)
	}
	t.Rebuild(true)
}
