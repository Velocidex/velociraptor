'use strict';

goog.module('grrUi.forms.jsonProxyDirective');
goog.module.declareLegacyNamespace();


const JsonProxyController = function($scope, $rootScope) {
    this.scope_ = $scope;
    this.rootScope_ = $rootScope;

    this.proxy;

    this.scope_.$watch('controller.proxy', this.onProxyChange_.bind(this), true);

    this.onValueChange_(this.scope_.value);
};


JsonProxyController.prototype.onValueChange_ = function(newValue, oldValue) {
    if (angular.isUndefined(newValue) || newValue == oldValue) {
        return;
    }

    // JSON.parse can not parse strings.
    if (this.scope_.type == "string" && this.proxy != newValue) {
        this.proxy = newValue;
    } else {
        try {
            var serialized = JSON.parse(newValue);
            if (serialized != this.proxy) {
                this.proxy = serialized;
            }
        } catch(ex) {
            if (this.proxy != newValue) {
                this.proxy = newValue;
            }
        }
    }
};

JsonProxyController.prototype.onProxyChange_ = function(newValue, oldValue) {
    if (angular.isUndefined(newValue) || newValue == oldValue) {
        return;
    }

    var serialized = JSON.stringify(newValue);
    // Simple strings do not JSON encoded.
    if (this.scope_.type == "string") {
        serialized = newValue;
    }

    if (serialized != this.scope_.value) {
        this.scope_.value = serialized;
    }
};


exports.JsonProxyDirective = function() {
  return {
    restrict: 'E',
      scope: {
          value: '=?',
          type: '@',
      },
      templateUrl: '/static/angular-components/forms/' +
          'json-proxy.html',
      controller: JsonProxyController,
      controllerAs: 'controller'
  };
};


exports.JsonProxyDirective.directive_name = 'grrJsonProxy';
