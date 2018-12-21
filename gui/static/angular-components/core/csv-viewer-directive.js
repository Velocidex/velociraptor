'use strict';

goog.module('grrUi.core.csvViewerDirective');
goog.module.declareLegacyNamespace();


// Angular is too slow to work with more rows.
var MAX_ROWS_PER_TABLE = 500;


/**
 * Controller for CSVViewerDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const CsvViewerDirective = function(
    $scope, grrApiService, DTOptionsBuilder, DTColumnBuilder) {

    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    this.DTOptionsBuilder = DTOptionsBuilder;
    this.DTColumnBuilder = DTColumnBuilder;

    /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    this.baseUrl;
    this.params;

    /** @type {?string} */
    this.pageData;

    this.options = {
        "pagingType": "full_numbers"
    };

    this.scope_.$watchGroup(['baseUrl', 'params'],
                            this.onContextChange_.bind(this));

    this.dtInstance = {};
};

/**
 * Handles changes to the clientId and filePath.
 *
 * @private
 */
CsvViewerDirective.prototype.onContextChange_ = function(newValues, oldValues) {
    if (newValues != oldValues || this.pageData == null) {
        this.fetchText_();
    }
};

/**
 * Fetches the file content.
 *
 * @private
 */
CsvViewerDirective.prototype.fetchText_ = function() {
    if (this.scope_.baseUrl && this.scope_.params) {
        var url = this.scope_.baseUrl;
        var params = this.scope_.params;
        params['start_row'] = 0;
        params['rows'] = MAX_ROWS_PER_TABLE;

        var self = this;
        if (angular.isDefined(this.dtInstance.DataTable)) {
            this.dtInstance.DataTable.ngDestroy();
            var i, ths = document.querySelectorAll('#dtable th');
            for (i=0;i<ths.length;i++) {
                ths[i].removeAttribute('style');
            }
        }
        this.pageData = null;
        this.grrApiService_.get(url, params).then(function(response) {
            self.pageData = response.data;
        }.bind(this), function() {
            this.pageData = null;
        }.bind(this)).catch(function() {
            this.pageData = null;
        });
    }
};

/**
 * CsvViewerDirective definition.
 * @return {angular.Directive} Directive definition object.
 */
exports.CsvViewerDirective = function() {
  return {
      scope: {
          baseUrl: '=',
          params: '=',
      },
      restrict: 'E',
      templateUrl: '/static/angular-components/core/csv-viewer.html',
    controller: CsvViewerDirective,
    controllerAs: 'controller',
  };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.CsvViewerDirective.directive_name = 'grrCsvViewer';
