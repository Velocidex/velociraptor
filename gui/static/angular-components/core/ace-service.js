'use strict';

goog.module('grrUi.core.aceService');
goog.module.declareLegacyNamespace();

var ace_options = null;

exports.AceService = function(grrApiService) {
    this.grrApiService_ = grrApiService;
};

exports.AceService.prototype.SaveAceConfig = function(ace) {
    if (angular.isObject(ace)) {
        window.ace_options = ace.getOptions();
    }
};

exports.AceService.prototype.AceConfig = function(ace) {
    var self = this;

    var ace_options = window.ace_options;

    if (angular.isObject(ace_options)) {
        for (let key in ace_options) {
            if (key == 'mode' || key == 'readOnly') {
                continue;
            }

            var value = ace_options[key];
            if (!angular.isUndefined(value) && value !== null) {
                ace.setOption(key, value);
            }
        }
        return;
    }

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

    ace.setOptions({
        enableBasicAutocompletion: true,
        enableLiveAutocompletion: true
    });
};

exports.AceService.service_name = 'grrAceService';
