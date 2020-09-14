'use strict';

goog.module('grrUi.core.searchBoxDirective');
goog.module.declareLegacyNamespace();


var ERROR_EVENT_NAME = 'ServerError';


/**
 * Controller for SearchBoxDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!angular.jQuery} $element
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @param {!grrUi.routing.routingService.RoutingService} grrRoutingService
 * @ngInject
 */
const SearchBoxController = function(
    $scope, $element, grrApiService, grrRoutingService) {
    var self = this;

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!angular.jQuery} */
    this.element_ = $element;

    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    /** @private {!grrUi.routing.routingService.RoutingService} */
    this.grrRoutingService_ = grrRoutingService;


    this.grrRoutingService_.uiOnParamsChanged(
        this.scope_, ['q'], function(unused_newValues, opt_stateParams) {
            self.query =  opt_stateParams["q"];
        });

    /** @export {string} */
    this.query = '';

    /** @export {Array} */
    this.labels = [];
};

/**
 * Updates UI with current query value.
 *
 * @export
 */
SearchBoxController.prototype.submitQuery = function(e) {
    if (this.scope_["navigate"]) {
        this.grrRoutingService_.go('search', {q: this.query});
        return;
    }

    this.scope_["query"] = this.query;

    e.preventDefault();
    e.stopPropagation();
    return false;
};

SearchBoxController.prototype.predict = function(viewValue) {
    var url = 'v1/SearchClients';
    var params = {
        query: this.query + "*",
        limit: 10,
        type: 1,
    };

    var self = this;
    this.grrApiService_.get(url, params).then(function (response) {
        self.labels = response.data.names;
    });
    return this.labels;
};

/**
 * Displays a table of clients.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.SearchBoxDirective = function() {
  return {
      scope: {
          query: "=",
          navigate: "=",
      },
      restrict: 'E',
      templateUrl: window.base_path+'/static/angular-components/core/search-box.html',
      controller: SearchBoxController,
      controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.SearchBoxDirective.directive_name = 'grrSearchBox';
