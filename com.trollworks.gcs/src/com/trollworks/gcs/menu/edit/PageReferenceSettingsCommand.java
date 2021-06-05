/*
 * Copyright ©1998-2021 by Richard A. Wilkes. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public
 * License, version 2.0. If a copy of the MPL was not distributed with
 * this file, You can obtain one at http://mozilla.org/MPL/2.0/.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as
 * defined by the Mozilla Public License, version 2.0.
 */

package com.trollworks.gcs.menu.edit;

import com.trollworks.gcs.menu.Command;
import com.trollworks.gcs.settings.PageReferenceSettingsWindow;
import com.trollworks.gcs.ui.UIUtilities;
import com.trollworks.gcs.utility.I18n;

import java.awt.event.ActionEvent;

public class PageReferenceSettingsCommand extends Command {
    /** The singleton {@link PageReferenceSettingsCommand}. */
    public static final PageReferenceSettingsCommand INSTANCE = new PageReferenceSettingsCommand();

    private PageReferenceSettingsCommand() {
        super(I18n.Text("Page Reference Settings…"), "ReferenceLookupSettings");
    }

    @Override
    public void adjust() {
        setEnabled(!UIUtilities.inModalState());
    }

    @Override
    public void actionPerformed(ActionEvent event) {
        PageReferenceSettingsWindow.display();
    }
}
