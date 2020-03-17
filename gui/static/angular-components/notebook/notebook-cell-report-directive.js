'use strict';

goog.module('grrUi.notebook.notebookCellReportDirective');
goog.module.declareLegacyNamespace();



const NotebookCellReportController = function(
    $scope, $compile, $element) {
    this.scope_ = $scope;

    this.element_ = $element;
    this.compile_ = $compile;
    this.template_ = "Loading ..";

    this.messages_;

    this.scope_.$watch('cell.timestamp',
                       this.onCellIdChange_.bind(this));

};

NotebookCellReportController.prototype.onCellIdChange_ = function() {
    var self = this;

    var cell = self.scope_["cell"];

    self.template_ = cell.output || "";
    self.messages_ = cell.messages || [];
    self.scope_["data"] = JSON.parse(cell.data || "{}");
    self.element_.html(self.template_).show();
    self.compile_(self.element_.contents())(self.scope_);
};


exports.NotebookCellReportDirective = function() {
  return {
      scope: {
          "cell": '=',
      },
      restrict: 'E',
      replace: true,
      linker: function(scope, element, attrs) {
          try {
              element.html(attrs.template_).show();
              attrs.compile_(attrs.element_.contents())(scope);
          } catch(err) {
              console.log(err);
          }
      },
      controller: NotebookCellReportController,
      controllerAs: 'controller'
  };
};

exports.NotebookCellReportDirective.directive_name = 'grrNotebookReportRenderer';
