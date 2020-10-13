'use strict';

goog.module('grrUi.hunt.huntOverviewDirective');
goog.module.declareLegacyNamespace();

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

    this.uiTraits = {};
    this.grrApiService_.getCached('v1/GetUserUITraits').then(function(response) {
        this.uiTraits = response.data['interface_traits'];
    }.bind(this), function(error) {
        if (error['status'] == 403) {
            this.error = 'Authentication Error';
        } else {
            this.error = error['statusText'] || ('Error');
        }
    }.bind(this));
};

HuntOverviewController.prototype.huntState = function(item) {
    if (item.stats.stopped) {
        return "STOPPED";
    }

    return item.state;
};


/**
 * Downloades the file.
 *
 * @export
 */
HuntOverviewController.prototype.prepareDownload = function(download_type) {
    var hunt = this.scope_["hunt"];
    var url = 'v1/CreateDownload';
    var params = {
        hunt_id: hunt.hunt_id,
    };

    if (download_type == 'summary') {
        params.only_combined_hunt = true;
    } else if(download_type == 'summary-json') {
        params.only_combined_hunt = true;
        params.json_format = true;
    } else if(download_type == 'summary-csv') {
        params.only_combined_hunt = true;
        params.csv_format = true;
    }

    this.grrApiService_.post(url, params).then(
        function success() {}.bind(this),
    );
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
    templateUrl: window.base_path+'/static/angular-components/hunt/hunt-overview.html',
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
