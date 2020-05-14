'use strict';

goog.module('grrUi.artifact.addArtifactDirective');


const AddArtifactController = function($scope, grrApiService) {
    this.scope_ = $scope;
    this.grrApiService_ = grrApiService;

    var self = this;
    this.scope_.aceConfig = function(ace) {
        self.ace = ace;
        ace.commands.addCommand({
            name: 'saveAndExit',
            bindKey: {win: 'Ctrl-Enter',  mac: 'Command-Enter'},
            exec: function(editor) {
                self.saveArtifact();
            },
        });

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
};


AddArtifactController.prototype.saveArtifact = function() {
    var url = 'v1/SetArtifactFile';
    var params = {
        artifact: this.scope_["artifact"],
    };

    this.grrApiService_.post(url, params).then(function(response) {
        if (response.data.error) {
            this.error = response.data['error_message'];
        } else {
            var onResolve = this.scope_['onResolve'];
            if (angular.isDefined(onResolve)) {
                onResolve();
            };
        }
    }.bind(this), function(error) {
        this.error = error;
    }.bind(this));
};

exports.AddArtifactDirective = function() {
  return {
      restrict: 'E',
      scope: {
          artifact: "=",
          onResolve: '&',
      },
      templateUrl: '/static/angular-components/artifact/' +
          'add_artifact.html',
      controller: AddArtifactController,
      controllerAs: 'controller'
  };
};

exports.AddArtifactDirective.directive_name = 'grrAddArtifact';
