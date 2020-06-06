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

exports.AceService.prototype.distill = function(previous) {
    // Search backwards from the end for various contexts - the winner is the last one.
    var regex = [/"[^"]*"/, /'[^']*'/, /`[^`]*`/, // /[0-9_\.a-zA-Z]+\([^\(\)]*\)/,
                 /\([^\(\)]*\)/, /\{[^\{\}]*\}/, /\[[^\[\]]*\]/];

    for (var i = 0; i< regex.length; i++) {
        if (previous.search(regex[i]) >= 0) {
            return previous.replace(regex[i], "");
        }
    }

    return previous;
};

exports.AceService.prototype.guessContext = function(previous, prefix) {
    // Simplify the previous string to eliminate completed
    // expressions.
    for(var i=0; i<30;i++) {
        var distilled = this.distill(previous);
        if (distilled == previous) {
            break;
        };
        previous = distilled;
    }

    // Now try to detect the present expression by looking to see
    // which construction is not completed at the end of the sentence.
    var results = [];

    // Strings
    var match = /"[^"]*?$/.exec(previous);
    if (match) {
        results.push({context: "string", pos: match.index});
    }

    match = /'[^']*?$/.exec(previous);
    if (match) {
        results.push({context: "string", pos: match.index});
    }

    // Plugin args - pos is the opening bracket.
    match = /(FROM +([a-z0-9A-Z_.]+))\([^\(\)]*?$/gi.exec(previous);
    if (match) {
        results.push({context: "plugin_args", pos: match.index + match[1].length,
                     name: match[2]});
    }

    // Plugin name
    match = /FROM +$/gi.exec(previous);
    if (match) {
        results.push({context: "plugin", pos: match.index});
    }

    // Function args - pos is the opening bracket.
    match = /([a-z0-9A-Z_.]+)\([^\(\)]*?$/i.exec(previous);
    if (match) {
        results.push({context: "function_args", pos: match.index + match[1].length,
                     name: match[1]});
    }

    // WHERE clause follows the plugin
    match = /FROM .+?$/gi.exec(previous);
    if (match) {
        results.push({context: "where", pos: match.index});
    }

    // WHERE clause content is after WHERE
    match = /WHERE .+?$/gi.exec(previous);
    if (match) {
        results.push({context: "where_clause", pos: match.index});
    }

    // Subquery
    match = /\{[^\}]*?$/.exec(previous);
    if (match) {
        results.push({context: "subquery", pos: match.index});
    }

    var result = {pos:0};
    for (var i=0; i<results.length; i++) {
        if (results[i].pos > result.pos) {
            result = results[i];
        };
    }
    return result;
};


exports.AceService.prototype.getKeywordCompletions = function(prefix) {
    var self = this;
    var completions = [];
    for (var i =0; i<self.completions.length; i++) {
        var item = self.completions[i];
        if (item.type == "Keyword") {
            completions.push({
                caption: item.name,
                description: item.description,
                snippet: item.name + " ",
                score: 200,
                value: item.name,
                meta: item.type,
            });
        }
    }
    return completions;
}

exports.AceService.prototype.getPluginCompletions = function(prefix) {
    var self = this;
    var completions = [];
    for (var i =0; i<self.completions.length; i++) {
        var item = self.completions[i];
        if (item.type == "Plugin") {
            var item_name = item.name;
            if (prefix == "?") {
                item_name = prefix + item_name;
            } else if (!item_name.startsWith(prefix)) {
                continue;
            }

            completions.push({
                caption: item_name,
                description: item.description || null,
                score: 1000,
                snippet: item.name + "(",
                type: "plugin",
                value: item.name,
                meta: item.type,
                docHTML: '<div class="text-wrap">' + item.description + "</div>",
            });
        }
    }
    return completions;
};

exports.AceService.prototype.getArtifactCompletions = function(prefix) {
    var self = this;
    var completions = [];
    for (var i =0; i<self.completions.length; i++) {
        var item = self.completions[i];
        if (item.type == "Artifact") {
            var item_name = item.name;
            if (!item_name.startsWith(prefix)) {
                continue;
            }

            var html = null;

            var components = item_name.split(".");
            var prefix_components = prefix.split(".");
            var current_component = components[prefix_components.length-1];
            var replacement = components.slice(0, prefix_components.length).join(".");

            if (components.length == prefix_components.length) {
                replacement += "(";

                if (item.description) {
                    html = '<div class="text-wrap">' + item.description + "</div>";
                }
            }

            completions.push({
                caption: replacement,
                description: item.name || null,
                score: 100,
                snippet: replacement,
                type: "Artifact",
                value: item.name,
                meta: item.type,
                docHTML: html,
            });
        }
    }
    return completions;
};


exports.AceService.prototype.getPluginArgsCompletions = function(name, prefix) {
    var self = this;
    var completions = [];
    for (var i =0; i<self.completions.length; i++) {
        var item = self.completions[i];
        if ((item.type == "Plugin" || item.type == "Artifact") && item.name == name) {
            var arg_desc = item.args || [];

            for (var j =0; j <arg_desc.length; j++) {
                var arg = item.args[j];
                var arg_name = arg.name;
                if (prefix == "?") {
                    arg_name = prefix + arg_name;
                } else if (!arg_name.startsWith(prefix)) {
                    continue;
                }

                var meta =  "plugin arg (" + arg.type + ")";
                if (item.type == "Artifact") {
                    meta = arg.type;
                };

                completions.push({
                    caption: arg_name,
                    description: arg.description,
                    snippet: arg.name + "=",
                    type: "argument",
                    value: arg.name,
                    score: 1000,
                    meta: meta,
                    docHTML: '<div class="text-wrap">' + arg.description + "</div>",
                });
            }
        }
    }
    return completions;
};


exports.AceService.prototype.getFunctionCompletions = function(prefix) {
    var self = this;
    var completions = [];
    for (var i =0; i<self.completions.length; i++) {
        var item = self.completions[i];
        if (item.type == "Function") {
            var item_name = item.name;
            if (prefix == "?") {
                item_name = prefix + item_name;
            } else if (!item_name.startsWith(prefix)) {
                continue;
            }

            var html = "";
            if (item.description) {
                html = '<div class="text-wrap">' + item.description + "</div>"
            }

            completions.push({
                caption: item_name,
                description: item.description,
                snippet: item.name + "(",
                type: "function",
                value: item.name,
                score: 10,
                meta: "function arg (" + item.type + ")",
                docHTML: html,
            });
        }
    }
    return completions;
};

exports.AceService.prototype.getFunctionArgsCompletions = function(name, prefix) {
    var self = this;
    var completions = [];
    for (var i =0; i<self.completions.length; i++) {
        var item = self.completions[i];
        if (item.type == "Function" && item.name == name) {
            var arg_desc = item.args || [];

            for (var j =0; j <arg_desc.length; j++) {
                var arg = item.args[j];
                var arg_name = arg.name;
                if (prefix == "?") {
                    arg_name = prefix + arg_name;
                } else if (!arg_name.startsWith(prefix)) {
                    continue;
                }

                completions.push({
                    caption: arg_name,
                    description: arg.description || null,
                    snippet: arg.name + "=",
                    type: "argument",
                    score: 1000,
                    value: arg.name,
                    meta: arg.type,
                    docHTML: "<h1>" + arg.description + "</h1>",
                });
            }
        }
    }
    return completions;
};


exports.AceService.prototype.initializeAceEditor = function(ace, options) {
    var self = this;

    self.grrApiService_.getCached('v1/GetKeywordCompletions').then(function(response) {
        self.completions = response.data['items'];
    });

    // create a completer object with a required callback function:
    var vqlCompleter = {
        identifierRegexps: [/[a-zA-Z_0-9.?\$\-\u00A2-\uFFFF]/],

        getCompletions: function(editor, session, pos, prefix, callback) {
            var previous_rows = session.doc.getAllLines().slice(0, pos.row+1);
            var last_idx = previous_rows.length-1;
            previous_rows[last_idx] = previous_rows[last_idx].slice(
                0, pos.column - prefix.length);

            var previous = previous_rows.join("");
            var context = self.guessContext(previous, prefix);

            // Do not complete inside a string.
            if (context.context == "string") {
                callback(null, []);

            } else if (context.context == "plugin") {
                callback(null, self.getPluginCompletions(prefix).concat(
                    self.getArtifactCompletions(prefix)));

            } else if (context.context == "plugin_args") {
                callback(null, self.getPluginArgsCompletions(context.name, prefix).concat(
                    self.getFunctionCompletions(prefix)));

            } else if (context.context == "function_args") {
                callback(null, self.getFunctionArgsCompletions(context.name, prefix).concat(
                    self.getFunctionCompletions(prefix)));

            } else {
                callback(null, self.getKeywordCompletions(prefix).concat(
                    self.getFunctionCompletions(prefix)));
            }
        }
    };
    var langTools = window.ace.require('ace/ext/language_tools');

    // finally, bind to langTools:
    langTools.setCompleters();
    langTools.addCompleter(vqlCompleter);
    langTools.addCompleter(langTools.textCompleter);

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
