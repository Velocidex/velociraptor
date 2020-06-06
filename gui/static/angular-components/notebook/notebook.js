'use strict';

goog.module('grrUi.notebook.notebook');
goog.module.declareLegacyNamespace();

const {NotebookDirective} = goog.require('grrUi.notebook.notebookDirective');
const {NotebookNewNotebookDialog} = goog.require('grrUi.notebook.notebookNewNotebookDialog');
const {NotebookExportNotebookDialog} = goog.require('grrUi.notebook.notebookExportNotebookDialog');
const {NotebookListDirective} = goog.require('grrUi.notebook.notebookListDirective');
const {NotebookRendererDirective} = goog.require('grrUi.notebook.notebookRendererDirective');
const {NotebookCellRendererDirective} = goog.require('grrUi.notebook.notebookCellRendererDirective');
const {NotebookCellReportDirective} = goog.require('grrUi.notebook.notebookCellReportDirective');
const {NewCellFromFlowModelDirective} = goog.require('grrUi.notebook.newCellFromFlowModelDirective');

/**
 * Angular module for notebook related UI.
 */
exports.notebookModule = angular.module('grrUi.notebook', [
    'ui.ace',
]);

exports.notebookModule.directive(
    NotebookDirective.directive_name, NotebookDirective);

exports.notebookModule.directive(
    NotebookListDirective.directive_name, NotebookListDirective);

exports.notebookModule.directive(
    NotebookRendererDirective.directive_name, NotebookRendererDirective);

exports.notebookModule.directive(
    NotebookCellRendererDirective.directive_name, NotebookCellRendererDirective);

exports.notebookModule.directive(
    NotebookNewNotebookDialog.directive_name, NotebookNewNotebookDialog);

exports.notebookModule.directive(
    NotebookExportNotebookDialog.directive_name, NotebookExportNotebookDialog);

exports.notebookModule.directive(
    NotebookCellReportDirective.directive_name, NotebookCellReportDirective);

exports.notebookModule.directive(
    NewCellFromFlowModelDirective.directive_name, NewCellFromFlowModelDirective);
