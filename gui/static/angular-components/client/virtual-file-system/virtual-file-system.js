'use strict';

goog.module('grrUi.client.virtualFileSystem.virtualFileSystem');
goog.module.declareLegacyNamespace();

const {AddItemButtonDirective} = goog.require('grrUi.client.virtualFileSystem.addItemButtonDirective');
const {BreadcrumbsDirective} = goog.require('grrUi.client.virtualFileSystem.breadcrumbsDirective');
const {DownloadDirective} = goog.require('grrUi.client.virtualFileSystem.downloadDirective');
const {FileContextDirective} = goog.require('grrUi.client.virtualFileSystem.fileContextDirective');
const {FileDetailsDirective} = goog.require('grrUi.client.virtualFileSystem.fileDetailsDirective');
const {FileHexViewDirective} = goog.require('grrUi.client.virtualFileSystem.fileHexViewDirective');
const {FileStatsViewDirective} = goog.require('grrUi.client.virtualFileSystem.fileStatsViewDirective');
const {FileTableDirective} = goog.require('grrUi.client.virtualFileSystem.fileTableDirective');
const {FileTextViewDirective} = goog.require('grrUi.client.virtualFileSystem.fileTextViewDirective');
const {FileTreeDirective} = goog.require('grrUi.client.virtualFileSystem.fileTreeDirective');
const {FileViewDirective} = goog.require('grrUi.client.virtualFileSystem.fileViewDirective');
const {RecursiveListButtonDirective} = goog.require('grrUi.client.virtualFileSystem.recursiveListButtonDirective');
const {VfsFilesArchiveButtonDirective} = goog.require('grrUi.client.virtualFileSystem.vfsFilesArchiveButtonDirective');
const {coreModule} = goog.require('grrUi.core.core');

/**
 * Angular module for clients-related UI.
 */
exports.virtualFileSystemModule = angular.module(
  'grrUi.client.virtualFileSystem',
  [coreModule.name, 'ui.ace', 'ui.bootstrap']);

exports.virtualFileSystemModule.directive(
    AddItemButtonDirective.directive_name, AddItemButtonDirective);
exports.virtualFileSystemModule.directive(
    BreadcrumbsDirective.directive_name, BreadcrumbsDirective);
exports.virtualFileSystemModule.directive(
    DownloadDirective.directive_name, DownloadDirective);
exports.virtualFileSystemModule.directive(
    FileContextDirective.directive_name, FileContextDirective);
exports.virtualFileSystemModule.directive(
    FileDetailsDirective.directive_name, FileDetailsDirective);
exports.virtualFileSystemModule.directive(
    FileHexViewDirective.directive_name, FileHexViewDirective);
exports.virtualFileSystemModule.directive(
    FileStatsViewDirective.directive_name, FileStatsViewDirective);
exports.virtualFileSystemModule.directive(
    FileTableDirective.directive_name, FileTableDirective);
exports.virtualFileSystemModule.directive(
    FileTextViewDirective.directive_name, FileTextViewDirective);
exports.virtualFileSystemModule.directive(
    FileTreeDirective.directive_name, FileTreeDirective);
exports.virtualFileSystemModule.directive(
    FileViewDirective.directive_name, FileViewDirective);
exports.virtualFileSystemModule.directive(
    RecursiveListButtonDirective.directive_name, RecursiveListButtonDirective);
exports.virtualFileSystemModule.directive(
    VfsFilesArchiveButtonDirective.directive_name,
    VfsFilesArchiveButtonDirective);
