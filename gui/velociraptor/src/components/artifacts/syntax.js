import api from '../core/api-service.js';

import language_tools from 'ace-builds/src-min-noconflict/ext-language_tools.js';


// This is a syntax editor class controlling suggestions from the
// server.
export default class Completer {
    state = {
        completions: [],
    }

    distill = (previous) => {
        // Search backwards from the end for various contexts - the winner is the last one.
        var regex = [/"[^"]*"/, /'[^']*'/, /`[^`]*`/, // /[0-9_\.a-zA-Z]+\([^\(\)]*\)/,
                     /\([^()]*\)/, /\{[^{}]*\}/, /\[[^[\]]*\]/];

        for (var i = 0; i< regex.length; i++) {
            if (previous.search(regex[i]) >= 0) {
                return previous.replace(regex[i], "");
            }
        }

        return previous;
    };

    guessContext = (previous, prefix) => {
        // Simplify the previous string to eliminate completed
        // expressions.
        for(var i=0; i<30;i++) {
            var distilled = this.distill(previous);
            if (distilled === previous) {
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
        match = /(FROM +([a-z0-9A-Z_.]+))\([^()]*?$/gi.exec(previous);
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
        match = /([a-z0-9A-Z_.]+)\([^()]*?$/i.exec(previous);
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
        match = /\{[^}]*?$/.exec(previous);
        if (match) {
            results.push({context: "subquery", pos: match.index});
        }

        var result = {pos:0};
        for (let i=0; i<results.length; i++) {
            if (results[i].pos > result.pos) {
                result = results[i];
            };
        }
        return result;
    };


    getKeywordCompletions = (prefix) => {
        var completions = [];
        for (var i =0; i<this.state.completions.length; i++) {
            var item = this.state.completions[i];
            if (item.type === "Keyword") {
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

    getPluginCompletions = (prefix) => {
        var completions = [];
        for (var i =0; i<this.state.completions.length; i++) {
            var item = this.state.completions[i];
            if (item.type === "Plugin") {
                var item_name = item.name;
                if (prefix === "?") {
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

    getArtifactCompletions = (prefix) => {
        var completions = [];
        for (var i =0; i<this.state.completions.length; i++) {
            var item = this.state.completions[i];
            if (item.type === "Artifact") {
                var item_name = item.name;
                if (!item_name.startsWith(prefix)) {
                    continue;
                }

                var html = null;

                var components = item_name.split(".");
                var prefix_components = prefix.split(".");
                var replacement = components.slice(0, prefix_components.length).join(".");

                if (components.length === prefix_components.length) {
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


    getPluginArgsCompletions = (name, prefix) => {
        var completions = [];
        for (var i =0; i<this.state.completions.length; i++) {
            var item = this.state.completions[i];
            if ((item.type === "Plugin" || item.type === "Artifact") && item.name === name) {
                var arg_desc = item.args || [];

                for (var j =0; j <arg_desc.length; j++) {
                    var arg = item.args[j];
                    var arg_name = arg.name;
                    if (prefix === "?") {
                        arg_name = prefix + arg_name;
                    } else if (!arg_name.startsWith(prefix)) {
                        continue;
                    }

                    var meta =  "plugin arg (" + arg.type + ")";
                    if (item.type === "Artifact") {
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


    getFunctionCompletions = (prefix) => {
        var completions = [];
        for (var i =0; i<this.state.completions.length; i++) {
            var item = this.state.completions[i];
            if (item.type === "Function") {
                var item_name = item.name;
                if (prefix === "?") {
                    item_name = prefix + item_name;
                } else if (!item_name.startsWith(prefix)) {
                    continue;
                }

                var html = "";
                if (item.description) {
                    html = '<div class="text-wrap">' + item.description + "</div>";
                }

                completions.push({
                    caption: item_name,
                    description: item.description,
                    snippet: item.name + "(",
                    type: "function",
                    value: item.name,
                    score: 10,
                    meta: "function",
                    docHTML: html,
                });
            }
        }
        return completions;
    };

    getFunctionArgsCompletions = (name, prefix) => {
        var completions = [];
        for (var i =0; i<this.state.completions.length; i++) {
            var item = this.state.completions[i];
            if (item.type === "Function" && item.name === name) {
                var arg_desc = item.args || [];

                for (var j =0; j <arg_desc.length; j++) {
                    var arg = item.args[j];
                    var arg_name = arg.name;
                    if (prefix === "?") {
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


    initializeAceEditor = (ace, options) => {
        api.get('v1/GetKeywordCompletions').then((response) => {
            this.state.completions = response.data['items'];
        });

        // create a completer object with a required callback function:
        var vqlCompleter = {
            identifierRegexps: [/[a-zA-Z_0-9.?$\-\u00A2-\uFFFF]/],

            getCompletions: (editor, session, pos, prefix, callback) => {
                var previous_rows = session.doc.getAllLines().slice(0, pos.row+1);
                var last_idx = previous_rows.length-1;
                previous_rows[last_idx] = previous_rows[last_idx].slice(
                    0, pos.column - prefix.length);

                var previous = previous_rows.join("");
                var context = this.guessContext(previous, prefix);

                // Do not complete inside a string.
                if (context.context === "string") {
                    callback(null, []);

                } else if (context.context === "plugin") {
                    callback(null, this.getPluginCompletions(prefix).concat(
                        this.getArtifactCompletions(prefix)));

                } else if (context.context === "plugin_args") {
                    callback(null, this.getPluginArgsCompletions(context.name, prefix).concat(
                        this.getFunctionCompletions(prefix)));

                } else if (context.context === "function_args") {
                    callback(null, this.getFunctionArgsCompletions(context.name, prefix).concat(
                        this.getFunctionCompletions(prefix)));

                } else {
                    callback(null, this.getKeywordCompletions(prefix).concat(
                        this.getFunctionCompletions(prefix)));
                }
            }
        };

        // finally, bind to langTools:
        language_tools.setCompleters();
        language_tools.addCompleter(vqlCompleter);
        language_tools.addCompleter(language_tools.textCompleter);

        if (!options.enableLiveAutocompletion) {
            ace.setOptions({
                enableBasicAutocompletion: true,
                enableLiveAutocompletion: true,
            });
        };
    };
}
