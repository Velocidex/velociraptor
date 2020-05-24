'use strict';

goog.module('grrUi.client.buildCollectorDirective');
goog.module.declareLegacyNamespace();


const BuildCollectorController = function(
    $scope, grrApiService) {

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!ApiService} */
    this.grrApiService_ = grrApiService;
};

exports.BuildCollectorDirective = function() {
  return {
    scope: {
      selectedHuntId: '=?',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/client/build-collector.html',
    controller: BuildCollectorController,
    controllerAs: 'controller'
  };
};


exports.BuildCollectorDirective.directive_name = 'grrBuildCollector';
