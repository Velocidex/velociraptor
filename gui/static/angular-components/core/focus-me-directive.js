'use strict';

goog.module('grrUi.core.focusMeDirective');
goog.module.declareLegacyNamespace();

// https://stackoverflow.com/questions/14833326/how-to-set-focus-on-input-field
exports.FocusMeDirective = function($parse, $timeout) {
  return {
      link: function (scope, element, attrs) {
          var model = $parse(attrs.focusMe);
          scope.$watch(model, function (value) {
              if (value === true) {
                  $timeout(function () {
                      element[0].focus();
                  });
              }
          });
          element.bind('blur', function () {
              $timeout(function () {
                  scope.$apply(model.assign(scope, false));
              });
          });
      }
  };
};

exports.FocusMeDirective.directive_name = 'focusMe';
