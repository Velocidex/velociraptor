import React, { Component } from 'react';
import PropTypes from 'prop-types';

import AceEditor from "react-ace";

import 'ace-builds/webpack-resolver.js';
import 'ace-builds/src-min-noconflict/ext-beautify.js';
import 'ace-builds/src-min-noconflict/ext-code_lens.js';
import 'ace-builds/src-min-noconflict/ext-elastic_tabstops_lite.js';
import 'ace-builds/src-min-noconflict/ext-emmet.js';
import 'ace-builds/src-min-noconflict/ext-error_marker.js';
import 'ace-builds/src-min-noconflict/ext-keybinding_menu.js';
import 'ace-builds/src-min-noconflict/ext-language_tools.js';
import 'ace-builds/src-min-noconflict/ext-linking.js';
import 'ace-builds/src-min-noconflict/ext-modelist.js';
import 'ace-builds/src-min-noconflict/ext-options.js';
import 'ace-builds/src-min-noconflict/ext-prompt.js';
import 'ace-builds/src-min-noconflict/ext-rtl.js';
import 'ace-builds/src-min-noconflict/ext-searchbox.js';
import 'ace-builds/src-min-noconflict/ext-settings_menu.js';
import 'ace-builds/src-min-noconflict/ext-spellcheck.js';
import 'ace-builds/src-min-noconflict/ext-split.js';
import 'ace-builds/src-min-noconflict/ext-static_highlight.js';
import 'ace-builds/src-min-noconflict/ext-statusbar.js';
import 'ace-builds/src-min-noconflict/ext-textarea.js';
import 'ace-builds/src-min-noconflict/ext-themelist.js';
import 'ace-builds/src-min-noconflict/ext-whitespace.js';
import 'ace-builds/src-min-noconflict/theme-ambiance.js';
import 'ace-builds/src-min-noconflict/theme-chaos.js';
import 'ace-builds/src-min-noconflict/theme-chrome.js';
import 'ace-builds/src-min-noconflict/theme-clouds.js';
import 'ace-builds/src-min-noconflict/theme-clouds_midnight.js';
import 'ace-builds/src-min-noconflict/theme-cobalt.js';
import 'ace-builds/src-min-noconflict/theme-crimson_editor.js';
import 'ace-builds/src-min-noconflict/theme-dawn.js';
import 'ace-builds/src-min-noconflict/theme-dracula.js';
import 'ace-builds/src-min-noconflict/theme-dreamweaver.js';
import 'ace-builds/src-min-noconflict/theme-eclipse.js';
import 'ace-builds/src-min-noconflict/theme-github.js';
import 'ace-builds/src-min-noconflict/theme-gob.js';
import 'ace-builds/src-min-noconflict/theme-gruvbox.js';
import 'ace-builds/src-min-noconflict/theme-idle_fingers.js';
import 'ace-builds/src-min-noconflict/theme-iplastic.js';
import 'ace-builds/src-min-noconflict/theme-katzenmilch.js';
import 'ace-builds/src-min-noconflict/theme-kr_theme.js';
import 'ace-builds/src-min-noconflict/theme-kuroir.js';
import 'ace-builds/src-min-noconflict/theme-merbivore.js';
import 'ace-builds/src-min-noconflict/theme-merbivore_soft.js';
import 'ace-builds/src-min-noconflict/theme-mono_industrial.js';
import 'ace-builds/src-min-noconflict/theme-monokai.js';
import 'ace-builds/src-min-noconflict/theme-nord_dark.js';
import 'ace-builds/src-min-noconflict/theme-pastel_on_dark.js';
import 'ace-builds/src-min-noconflict/theme-solarized_dark.js';
import 'ace-builds/src-min-noconflict/theme-solarized_light.js';
import 'ace-builds/src-min-noconflict/theme-sqlserver.js';
import 'ace-builds/src-min-noconflict/theme-terminal.js';
import 'ace-builds/src-min-noconflict/theme-textmate.js';
import 'ace-builds/src-min-noconflict/theme-tomorrow.js';
import 'ace-builds/src-min-noconflict/theme-tomorrow_night_blue.js';
import 'ace-builds/src-min-noconflict/theme-tomorrow_night_bright.js';
import 'ace-builds/src-min-noconflict/theme-tomorrow_night_eighties.js';
import 'ace-builds/src-min-noconflict/theme-tomorrow_night.js';
import 'ace-builds/src-min-noconflict/theme-twilight.js';
import 'ace-builds/src-min-noconflict/theme-vibrant_ink.js';
import 'ace-builds/src-min-noconflict/theme-xcode.js';
import 'ace-builds/src-min-noconflict/keybinding-emacs.js';
import 'ace-builds/src-min-noconflict/keybinding-sublime.js';
import 'ace-builds/src-min-noconflict/keybinding-vim.js';
import 'ace-builds/src-min-noconflict/keybinding-vscode.js';
import  'ace-builds/src-min-noconflict/mode-yaml.js';
import  'ace-builds/src-min-noconflict/mode-json.js';
import  'ace-builds/src-min-noconflict/mode-markdown.js';
import  'ace-builds/src-min-noconflict/mode-sql.js';

export default class VeloAce extends Component {
    static propTypes = {
        text: PropTypes.string,
        mode: PropTypes.string,
        onChange: PropTypes.func,
    }

    render() {
        return (
            <AceEditor
              mode={this.props.mode || 'sql'}
              theme="github"
              value={this.props.text || ''}
              onChange={this.props.onChange}
              editorProps={{
                  $blockScrolling: true,
              }}
            />
        );
    }
}
