#!/usr/bin/env python

import os
import subprocess
from gi.repository import Nautilus, GObject, Gtk
from xml.sax.saxutils import escape


def zenity_dialog(typ, title, message):
    dialog = Gtk.MessageDialog(
        None, 0, typ,
        Gtk.ButtonsType.OK,
        title,
    )
    dialog.format_secondary_text(
        message
    )
    dialog.run()
    dialog.destroy()


def show_simulation_results(results):
    title = "Takeown simulation results"
    try:
        message = str(results)
    except UnicodeEncodeError:
        message = unicode(results).encode("utf-8")
    zenity_dialog(Gtk.MessageType.INFO, title, message)


def error_running_takeown(error):
    title = "Error running takeown"
    try:
        message = str(error)
    except UnicodeEncodeError:
        message = unicode(error).encode("utf-8")
    zenity_dialog(Gtk.MessageType.ERROR, title, message)


class TakeownItemExtension(GObject.GObject, Nautilus.MenuProvider):
    '''Display takeown context items on selected files.'''

    def get_file_items(self, window, files):
        '''Attaches context menu in Nautilus
        '''
        if not files:
            return

        menu_item = Nautilus.MenuItem(
            name='TakeownMenuProvider::Takeown',
            label='Take ownership',
            tip='',
            icon=''
        )
        r_menu_item = Nautilus.MenuItem(
            name='TakeownMenuProvider::TakeownRecursively',
            label='Take ownership recursively',
            tip='',
            icon=''
        )
        rs_menu_item = Nautilus.MenuItem(
            name='TakeownMenuProvider::SimulateTakeownRecursively',
            label='Simulate recursive taking of ownership',
            tip='',
            icon=''
        )

        menu_item.connect('activate', self.on_menu_item_clicked, files)
        r_menu_item.connect('activate', self.on_menu_item_clicked, files)
        rs_menu_item.connect('activate', self.on_menu_item_clicked, files)
        return menu_item, r_menu_item, rs_menu_item

    def on_menu_item_clicked(self, menu, files):
        '''Called when any takeown menu is clicked.'''
        paths = []
        for file_obj in files:
            if file_obj.is_gone():
                continue
            paths.append(
              os.path.relpath(
                file_obj.get_location().get_path(),
                os.getcwd()
              )
            )
        if not paths:
            return

        cmd = ["takeown"]
        if "Recursively" in menu.get_property('name'):
            cmd.append("-r")
        if "Simulate" in menu.get_property('name'):
            cmd.append("-s")
        cmd.append("--")

        try:
            text = subprocess.Popen(
                cmd + paths,
                stdin=None,
                stdout=subprocess.PIPE,
                stderr=subprocess.STDOUT
            ).communicate()[0]
            if text.strip():
                if "Simulate" in menu.get_property('name'):
                    show_simulation_results(text.strip())
                else:
                    error_running_takeown(text.strip())
        except OSError, e:
            error_running_takeown(e.strerror)
