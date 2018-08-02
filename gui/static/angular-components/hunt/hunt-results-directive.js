'use strict';

goog.module('grrUi.hunt.huntResultsDirective');
goog.module.declareLegacyNamespace();

const {downloadableVfsRoots, getPathSpecFromValue, makeValueDownloadable, pathSpecToAff4Path} = goog.require('grrUi.core.fileDownloadUtils');
const {stripAff4Prefix} = goog.require('grrUi.core.utils');



/**
 * Controller for HuntResultsDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @ngInject
 */
const HuntResultsController = function(
    $scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @export {string} */
  this.resultsUrl;

  /** @export {string} */
  this.exportedResultsUrl;

  $scope.$watch('huntId', this.onHuntIdChange.bind(this));
};


/**
 * Handles huntId attribute changes.
 *
 * @param {?string} huntId
 * @export
 */
HuntResultsController.prototype.onHuntIdChange = function(huntId) {
  if (!angular.isString(huntId)) {
    return;
  }
  this.queryParams = {'hunt_id': huntId};
  this.resultsUrl = '/v1/GetHuntResults';
  this.exportedResultsUrl = '/v1/DownloadHuntResults';
};

/**
 * Directive for displaying results of a hunt with a given ID.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.HuntResultsDirective = function() {
  return {
    scope: {
      huntId: '='
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/hunt/hunt-results.html',
    controller: HuntResultsController,
    controllerAs: 'controller'
  };
};


/**
 * Directive's name in Angular.
 *
 * @const
 * @export
 */
exports.HuntResultsDirective.directive_name = 'grrHuntResults';
