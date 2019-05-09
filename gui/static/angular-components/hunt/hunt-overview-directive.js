'use strict';

goog.module('grrUi.hunt.huntOverviewDirective');
goog.module.declareLegacyNamespace();

var ERROR_EVENT_NAME = 'ServerError';

/**
 * Controller for HuntOverviewDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const HuntOverviewController = function($scope, grrApiService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @type {string} */
    this.scope_.hunt;

    this.grrApiService_ = grrApiService;
};



/**
 * Downloades the file.
 *
 * @export
 */
HuntOverviewController.prototype.downloadFile = function() {
    var url = 'v1/DownloadHuntResults';
    var hunt = this.scope_["hunt"];
    if (angular.isDefined(hunt)) {
        var params = {hunt_id: hunt.hunt_id};
        this.grrApiService_.downloadFile(url, params).then(
            function success() {}.bind(this),
            function failure(response) {
                if (angular.isUndefined(response.status)) {
                    this.rootScope_.$broadcast(
                        ERROR_EVENT_NAME, {
                            message: 'Couldn\'t download file.'
                        });
                }
            }.bind(this)
        );
    }
};



/**
 * Directive for displaying log records of a hunt with a given URN.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.HuntOverviewDirective = function() {
  return {
    scope: {
      hunt: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/hunt/hunt-overview.html',
    controller: HuntOverviewController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntOverviewDirective.directive_name = 'grrHuntOverview';
