'use strict';

goog.module('grrUi.artifact.reportingDirective');

/**
 * Controller for ReportingDirective.
 *
 * @constructor
 * @param {!angular.Scope} $scope
 * @param {!grrUi.core.apiService.ApiService} grrApiService
 * @ngInject
 */
const ReportingController = function(
    $scope, $compile, $element, grrApiService) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;

  /** @private {!grrUi.core.apiService.ApiService} */
    this.grrApiService_ = grrApiService;

    this.element_ = $element;
    this.compile_ = $compile;
    this.template_ = "Loading ..";

    this.messages;

    this.scope_.$watch(
        'params',
        this.onContextChange_.bind(this), true);
};


/**
 * Handles changes to the clientId and filePath.
 *
 * @private
 */
ReportingController.prototype.onContextChange_ = function(newValues) {
    if (angular.isObject(newValues)) {
        var self = this;

        self.element_.html("Loading ...").show();

        this.grrApiService_.post(
            "/v1/GetReport", this.scope_.params).then(function(response) {
                self.template_ = response.data.template || "No Reports";
                self.messages = response.data.messages || [];
                for (var i=0; i<self.messages.length; i++) {
                    console.log("While generating report: " + self.messages[i]);
                }

                self.scope_["data"] = JSON.parse(response.data.data);
                self.element_.html(self.template_).show();
                self.compile_(self.element_.contents())(self.scope_);

            }.bind(this), function(err) {
                self.template_ = "Error " + err.data.message;
                self.element_.html(self.template_).show();
            }.bind(this)).catch(function(err) {
                self.template_ = "Error " + err.message;
                self.element_.html(self.template_).show();
            });
    }
};


/**
 * ReportingDirective definition.
 *
 * @return {angular.Directive} Directive definition object.
 */
exports.ReportingDirective = function() {
    return {
        restrict: 'E',
        scope: {
            params: '='
        },
        replace: true,
        linker: function(scope, element, attrs) {
            try {
                element.html(attrs.template_).show();
                attrs.compile_(attrs.element_.contents())(scope);
            } catch(err) {
                console.log(err);
            }
        },
        controller: ReportingController,
        controllerAs: 'controller',
    };
};


/**
 * Name of the directive in Angular.
 *
 * @const
 * @export
 */
exports.ReportingDirective.directive_name = 'grrReportViewer';
