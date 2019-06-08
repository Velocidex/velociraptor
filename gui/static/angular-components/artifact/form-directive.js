'use strict';

goog.module('grrUi.artifact.formDirective');

const FormController = function($scope) {
  /** @private {!angular.Scope} */
  this.scope_ = $scope;
  this.popup_opened = false;
  this.date_time;
  $scope.$watch('controller.date_time',
                this.onDateChange_.bind(this));
};


FormController.prototype.onDateChange_ = function() {
  if (angular.isDefined(this.date_time)) {
    this.scope_["value"] = (this.date_time.getTime() / 1000).toString();
  }
};

FormController.prototype.openDatePopup = function() {
  this.popup_opened = true;
};

exports.FormDirective = function() {
  return {
      restrict: 'E',
      scope: {
          value: '=',
          type: '=',
      },
      templateUrl: '/static/angular-components/artifact/form.html',
      controller: FormController,
      controllerAs: 'controller'
  };
};


exports.FormDirective.directive_name = 'grrForm';
