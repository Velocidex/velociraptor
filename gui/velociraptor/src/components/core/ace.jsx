import './ace.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';

import 'ace-builds';
import AceEditor from "react-ace";
import T from '../i8n/i8n.jsx';

// import 'ace-builds/webpack-resolver.js';
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
import 'ace-builds/src-min-noconflict/theme-one_dark.js';
import 'ace-builds/src-min-noconflict/theme-github_dark.js';
import 'ace-builds/src-min-noconflict/theme-github_light_default.js';
import 'ace-builds/src-min-noconflict/theme-cloud_editor_dark.js';
import 'ace-builds/src-min-noconflict/theme-cloud_editor.js';
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
import 'ace-builds/src-min-noconflict/mode-yaml.js';
import 'ace-builds/src-min-noconflict/mode-json.js';
import 'ace-builds/src-min-noconflict/mode-markdown.js';
import 'ace-builds/src-min-noconflict/mode-sql.js';

// Custom VQL syntax highlighter
import VqlMode from './mode-vql.jsx';
import MarkdownMode from './mode-markdown.jsx';
import YamlMode from './mode-yaml.jsx';
import SigmaMode from './mode-sigma.jsx';
import RegexMode from './mode-regex.jsx';
import YaraMode from './mode-yara.jsx';
import classNames from "classnames";

import ace from 'ace-builds/src-noconflict/ace.js';

// Only support the following modes.
ace.config.set('basePath', '/');
ace.config.$modes = {
    'ace/mode/vql': new VqlMode(),
    'ace/mode/md': new MarkdownMode(),
    'ace/mode/yaml': new YamlMode(),
    'ace/mode/sigma': new SigmaMode(),
    'ace/mode/regex': new RegexMode(),
    'ace/mode/yara': new YaraMode(),
};
_.each(ace.config.$modes, (v, k)=>{
    ace.define(k, null, v);
});

let options = window.ace.acequire("ace/ext/options");
options.optionGroups.Main.Mode.items = [
    {
        caption: "VQL",
        value: "ace/mode/vql",
    }, {
        caption: "YAML",
        value: "ace/mode/yaml",
    }, {
        caption: "Markdown",
        value: "ace/mode/md",
    }, {
        caption: "Sigma",
        value: "ace/mode/sigma",
    }, {
        caption: "Regex",
        value: "ace/mode/regex",
    },  {
        caption: "Yara",
        value: "ace/mode/yara",
    },
];

import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

import UserConfig from './user.jsx';

import ToolTip from '../widgets/tooltip.jsx';

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';
import { JSONparse } from '../utils/json_parse.jsx';

export class SettingsButton extends Component {
    static propTypes = {
        ace: PropTypes.object,
    }

    render() {
        return (
            <ToolTip tooltip={T("Configure Editor")}>
              <Button variant="default"
                      className="float-left"
                      onClick={() => this.props.ace.execCommand("showSettingsMenu")} >
                <FontAwesomeIcon icon="text-height"/>
        <span className="sr-only">{T("Configure Editor")}</span>
              </Button>
            </ToolTip>
        );
    }
}

export default class VeloAce extends Component {
    constructor() {
        super();
        this.aceRef= React.createRef();
    }

    static contextType = UserConfig;
    static propTypes = {
        text: PropTypes.string,
        mode: PropTypes.string,
        focus: PropTypes.bool,
        onChange: PropTypes.func,
        options: PropTypes.object,
        placeholder: PropTypes.string,

        // Extra toolbar buttons to go in the editor toolbar.
        toolbar: PropTypes.any,

        // Will be called with the underlying editor object when first
        // mounted for configuration.
        aceConfig: PropTypes.func,

        // If this is defined, we call it with the editor settings
        // button. Our caller can then place it where they want.
        settingButtonRenderer: PropTypes.func,

        commands: PropTypes.array,
        className: PropTypes.string,
    }

    // Remove options which are not settable by the user since they
    // are used by the components.
    normalizeOptions = (options) => {
        _.each(options, (v, k) => {
            switch(k) {
            case "mode":
            case "readOnly":
            case "minLines":
            case "maxLines":
            case "autoScrollEditorIntoView":
                delete options[k];
                break;
            default:
                break;
            }

            if (!v) {
                delete options[k];
            }
        });

        return options;
    }

    getUserOptions = () => {
        let user_options = this.normalizeOptions(
            JSONparse(this.context.traits.ui_settings, {}));
        return Object.assign(user_options, this.props.options || {});
    }

    updatePreferences = (e, editor) => {
        let new_options = this.normalizeOptions(editor.getOptions());

        // If options have changed we need to update them to the
        // server.
        if (!_.isEqual(new_options, this.getUserOptions())) {
            api.post("v1/SetGUIOptions",
                     {options: JSON.stringify(new_options)},
                     this.source.token).then((response) => {
                         this.context.updateTraits();
                     });
        }
    }

    state = {
        // The raw ace editor.
        ace: {},
        mode: "text",
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate() {
        let ref = this.aceRef.current;

        if (ref && this.props.mode !== this.state.mode) {
            this.setState({mode: this.props.mode});

            let sess = ref.editor.getSession();
            sess.setUseWorker(false);
            if (this.props.mode === "vql") {
                sess.setMode(new VqlMode());
            } else if(this.props.mode === "markdown") {
                sess.setMode(new MarkdownMode());
            } else if(this.props.mode === "yaml") {
                sess.setMode(new YamlMode());
            } else if(this.props.mode === "sigma") {
                sess.setMode(new SigmaMode());
            } else if(this.props.mode === "regex") {
                sess.setMode(new RegexMode());
            } else if(this.props.mode === "yara") {
                sess.setMode(new YaraMode());
            }
        }
    }

    render() {
        // Merge the user's options into the options the component
        // specified.
        let options = this.getUserOptions();
        let mode = this.props.mode || 'sql';
        let focus = this.props.focus;

        if (_.isUndefined(focus)) {
            focus = true;
        }
        return (
            <>
              <div
                className={classNames(
                  "col-12",
                  "velo-ace-editor",
                  this.props.className)}>
                <AceEditor
                  ref={this.aceRef}
                  className="full-height"
                  showGutter={true}
                  focus={focus}
                  mode={mode}
                  theme="github"
                  placeholder={this.props.placeholder}
                  value={this.props.text || ''}
                  onChange={this.props.onChange}
                  style={
                      {width: "100%"}
                  }
                  commands={this.props.commands}
                  setOptions={options}
                  editorProps={{
                      $blockScrolling: true,
                  }}
                  onBlur={this.updatePreferences}
                  onLoad={(ace) => {
                      this.setState({ace: ace});
                      if (this.props.aceConfig) {
                          this.props.aceConfig(ace);
                      }
                  }}
                />
              </div>
            </>
        );
    }
}
