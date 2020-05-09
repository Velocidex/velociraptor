'use strict';

goog.module('grrUi.forms.clientLabelFormDirective');
goog.module.declareLegacyNamespace();

const {ApiService} = goog.require('grrUi.core.apiService');


/**
 * Controller for ClientLabelFormDirective.
 *
 * @param {!angular.Scope} $scope
 * @param {!ApiService} grrApiService
 * @constructor
 * @ngInject
 */
const ClientLabelFormController = function($scope, grrApiService) {
    /** @private {!angular.Scope} */
    this.scope_ = $scope;

    /** @private {!ApiService} */
    this.grrApiService_ = grrApiService;

    /** @type {*} */
    this.labelsList = [];

    var params = {
        query: "label:*",
        limit: 100,
        type: 1,
    };
    this.grrApiService_.get('v1/SearchClients', params).then(function(response) {
        this.labelsList = [];
        var data = response['data']['names'];
        for (var i=0; i<data.length; i++) {
            this.labelsList.push(data[i].replace(/^label:/, ""));
        };

        this.labelsList.sort();
        if (this.labelsList.length > 0) {
            this.scope_.value["label"] = this.labelsList[0];
        }
    }.bind(this));
};


/**
 * Initializes the client label form controller.
 */
ClientLabelFormController.prototype.$onInit = function() {
  this.hideEmptyOption = this.hideEmptyOption || false;
  this.emptyOptionLabel = this.emptyOptionLabel || '-- All clients --';
};

/**
 * Directive that displays a client label selector.
 *
 * @return {!angular.Directive} Directive definition object.
 * @ngInject
 * @export
 */
exports.ClientLabelFormDirective = function() {
  return {
      scope: {
          value: '=',
    },
    restrict: 'E',
    templateUrl: '/static/angular-components/forms/client-label-form.html',
    controller: ClientLabelFormController,
    controllerAs: 'controller'
  };
};


/**
 * Name of the directive in Angular.
 */
exports.ClientLabelFormDirective.directive_name = 'grrFormClientLabel';
