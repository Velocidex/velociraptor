'use strict';

goog.module('grrUi.core.aceService');
goog.module.declareLegacyNamespace();

exports.AceService = function(grrApiService) {
    this.grrApiService_ = grrApiService;
};

exports.AceService.prototype.SaveAceConfig = function(ace) {
    if (angular.isObject(ace)) {
        window.ace_options = ace.getOptions();

        var params = {options: JSON.stringify(window.ace_options)};
        this.grrApiService_.post('v1/SetGUIOptions', params).then(function(response) {

        });
    }
};

var setAceOptions = function(ace, ace_options) {
    for (let key in ace_options) {
        // Ignore some options that depend on the widget.
        if (key == 'mode' ||
            key == 'readOnly' ||
            key == "minLines" ||
            key == "maxLines" ||
            key == "autoScrollEditorIntoView" ) {
            continue;
        }

        var value = ace_options[key];
        if (!angular.isUndefined(value) && value !== null) {
            ace.setOption(key, value);
        }
    }
};


exports.AceService.prototype.initializeAceEditor = function(ace, options) {
    var self = this;

    self.grrApiService_.getCached('v1/GetKeywordCompletions').then(function(response) {
        self.completions = response.data['items'];
    });

    // create a completer object with a required callback function:
    var vqlCompleter = {
        getCompletions: function(editor, session, pos, prefix, callback) {
            callback(null, self.completions.map(function(item) {
                return {
                    caption: item.name,
                    value: item.name,
                    meta: item.type,
                };
            }));
        }
    };
    var langTools = window.ace.require('ace/ext/language_tools');

    // finally, bind to langTools:
    langTools.setCompleters([vqlCompleter, langTools.textCompleter]);

    if (!angular.isDefined(options["enableLiveAutocompletion"])) {
        ace.setOptions({
            enableBasicAutocompletion: true,
            enableLiveAutocompletion: true,
        });
    };
};

exports.AceService.prototype.AceConfig = function(ace) {
    var self = this;

    // Take focus to the new editor.
    ace.focus();

    var ace_options = window.ace_options;

    if (angular.isObject(ace_options)) {
        setAceOptions(ace, ace_options);
    } else {
        self.grrApiService_.getCached('v1/GetUserUITraits').then(function(response) {
            self.uiTraits = response.data['interface_traits'];
            var options = self.uiTraits['ui_settings'] || "{}";
            window.ace_options = JSON.parse(options);
            setAceOptions(ace, window.ace_options);

            self.initializeAceEditor(ace, window.ace_options);
        });
    }

};

exports.AceService.service_name = 'grrAceService';
