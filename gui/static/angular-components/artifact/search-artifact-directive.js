'use strict';

goog.module('grrUi.artifact.searchArtifactDirective');

/**
 * Controller for SearchArtifactDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const SearchArtifactController = function(
    $scope, grrApiService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @export {Object<string, Object>} */
    this.descriptors = {};

    /** @export {string} */
    this.descriptorsError;

    /** @export {Object} */
    this.selectedName;

    // A list of descriptors that matched the search term.
    this.matchingDescriptors = [];

    this.reportParams = {};

    this.param_types = {};
    this.param_info = {};
    this.param_descriptions = {};
    this.paramDescriptors = {};

    this.search_focus = true;

    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    /** @export {string} */
    this.search = '';

    this.scope_.$watch('controller.search',
                       this.onSearchChange_.bind(this));

    this.scope_.$watch('names', this.onNamesChanged_.bind(this));
};

SearchArtifactController.prototype.onNamesChanged_ = function() {
    var self = this;
    if (this.scope_["names"].length>0) {
        this.selectArtifact(this.scope_["names"][0]);

        this.grrApiService_.get("v1/GetArtifacts", {names: this.scope_["names"]}).then(
            function(response) {
                var items = response['data'].items;
                for(var i=0; i < items.length;i++) {
                  var item = items[i];
                  self.descriptors[item.name] = item;

                  var params = item.parameters;
                  if (angular.isObject(params)) {
                    for (var j=0; j<params.length; j++) {
                        var param = params[j];

                        self.param_types[param.name] = param.type;
                        self.param_info[param.name] = param;
                    }
                  }
                }
            });
    }
};


/**
 * Adds artifact with a given name to the list of selected names.
 *
 * @param {string} name The name of the artifact to add to the
 *     selected list.
 * @export
 */
SearchArtifactController.prototype.add = function(name) {
  if (angular.isUndefined(name) || name == "") {
    return;
  }

  var self = this;
  var index = -1;
  for (var i = 0; i < self.scope_.names.length; ++i) {
    if (self.scope_.names[i] == name) {
      index = i;
      break;
    }
  }
  if (index == -1) {
    self.scope_.names.push(name);

    for (var i=0; i<self.scope_.names.length; i++) {
      var name = self.scope_.names[i];
      var params = self.descriptors[name].parameters;
      if (angular.isObject(params)) {
        for (var j=0; j<params.length; j++) {
          var param = params[j];

            if (!angular.isDefined(self.scope_.params[param.name])) {
                self.scope_.params[param.name]= param.default || "";
                self.param_types[param.name] = param.type;
                self.param_info[param.name] = param;
                self.param_descriptions[param.name] = param.description;
            }
        }
      }
    }
  }
};

/**
 * Removes given name from the list of selected artifacts names.
 *
 * @param {string} name The name to be removed from the list of
 *     selected names.
 * @export
 */
SearchArtifactController.prototype.remove = function(name) {
    var self = this;
    var index = -1;
    for (var i = 0; i < self.scope_.names.length; ++i) {
        if (self.scope_.names[i] == name) {
            index = i;
            break;
        }
    }

    if (index != -1) {
        self.scope_.names.splice(index, 1);

        var is_key_defined = function(key) {
            for (var i = 0; i < self.scope_.names.length; ++i) {
                var name = self.scope_.names[i];
                var params = self.descriptors[name].parameters || [];
                for (var j=0; j<params.length; j++) {
                    if (params[j].name == key) {
                        return true;
                    }
                }
            }
            return false;
        };

        for (var k in self.scope_.params) {
            if (self.scope_.params.hasOwnProperty(k) && !is_key_defined(k)) {
                delete self.scope_.params[k];
            }
        }
    }

  this.selectedName = null;
};

/**
 * Removes all names from the list of selected artifacts names.
 *
 * @export
 */
SearchArtifactController.prototype.clear = function() {
  angular.forEach(angular.copy(this.scope_.names), function(name) {
    this.remove(name);
  }.bind(this));
};

SearchArtifactController.prototype.selectArtifact = function(name) {
  this.selectedName = name;
  this.reportParams= {
    artifact: this.selectedName,
    type: "ARTIFACT_DESCRIPTION",
  };
};

SearchArtifactController.prototype.onSearchChange_ = function() {
    var self = this;
    this.grrApiService_.get(
        "/v1/GetArtifacts", {
            search_term: self.search,
            type: self.scope_["type"],
        }).then(
            function(response){
                self.matchingDescriptors = [];

                for(var i=0; i<response.data.items.length; i++) {
                    var desc = response.data.items[i];
                    self.descriptors[desc.name] = desc;
                    self.matchingDescriptors.push(desc);
                };
            }, function(err) {
                self.descriptorsError = err;
            });
};

exports.SearchArtifactDirective = function() {
  return {
      restrict: 'E',
      scope: {
          names: '=',
          params: '=',
          type: "=",
      },
      templateUrl: '/static/angular-components/artifact/' +
          'search-artifact.html',
      controller: SearchArtifactController,
      controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.SearchArtifactDirective.directive_name = 'grrSearchArtifact';
