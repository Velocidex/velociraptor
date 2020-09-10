'use strict';

var gulp = require('gulp');
var gulpAngularTemplateCache = require('gulp-angular-templatecache');
var gulpClosureCompiler = require('google-closure-compiler');
var gulpConcat = require('gulp-concat');
var gulpInsert = require('gulp-insert');
var gulpLess = require('gulp-less');
var gulpNewer = require('gulp-newer');
var gulpPlumber = require('gulp-plumber');
var sourcemaps = require('gulp-sourcemaps');
var uglify = require('gulp-uglify');
var del = require('del');

var config = {};
config.nodeModulesDir = './node_modules';
config.distDir = 'dist';
config.tempDir = 'tmp';

var isWatching = false;

var closureCompiler = gulpClosureCompiler.gulp();

const closureCompilerFlags = {
    compilation_level: 'WHITESPACE_ONLY',
  jscomp_off: [
    'checkTypes',
    'checkVars',
    'externsValidation',
    'invalidCasts',
  ],
  jscomp_error: [
    'const',
    'constantProperty',
    'globalThis',
    'missingProvide',
    'missingProperties',
    'missingRequire',
    'nonStandardJsDocs',
    'strictModuleDepCheck',
    'undefinedNames',
    'uselessCode',
    'visibility',
  ],
  language_out: 'ECMASCRIPT6_STRICT',
  // See https://github.com/google/closure-compiler/issues/1138 for details.
  force_inject_library: [
    'base',
    'es6_runtime',
  ],
  source_map_format: 'V3'
};


/**
 * Third-party tasks.
 */
gulp.task('compile-third-party-js', function() {
  return gulp.src([config.nodeModulesDir + '/jquery/dist/jquery.js',
                   config.nodeModulesDir + '/popper.js/dist/umd/popper.min.js',
                   // config.nodeModulesDir + '/jquery-migrate/dist/jquery-migrate.js',
                   config.nodeModulesDir + '/google-closure-library/closure/goog/base.js',
                   config.nodeModulesDir + '/bootstrap/dist/js/bootstrap.js',
                   config.nodeModulesDir + '/angular/angular.js',
                   config.nodeModulesDir + '/angular-animate/angular-animate.js',
                   config.nodeModulesDir + '/angular-cookies/angular-cookies.js',
                   config.nodeModulesDir + '/angular-resource/angular-resource.js',
                   config.nodeModulesDir + '/flot/dist/es5/jquery.flot.js',
                   config.nodeModulesDir + '/flot/source/jquery.flot.selection.js',
                   config.nodeModulesDir + '/flot/source/jquery.canvaswrapper.js',
                   config.nodeModulesDir + '/vis/dist/vis.min.js',
                   config.nodeModulesDir + '/moment/moment.js',
                   config.nodeModulesDir + '/angular-ui-bootstrap/dist/ui-bootstrap-tpls.js',
                   config.nodeModulesDir + '/angular-ui-router/release/angular-ui-router.js',
                   config.nodeModulesDir + '/ng-sanitize/index.js',
                   config.nodeModulesDir + '/ui-select/dist/select.min.js',
                   config.nodeModulesDir + '/datatables/media/js/jquery.dataTables.js',
                   config.nodeModulesDir + '/angular-datatables/dist/angular-datatables.js',
                   config.nodeModulesDir + '/datatables.net-colreorder/js/dataTables.colReorder.js',
                   config.nodeModulesDir + '/datatables.net-buttons/js/dataTables.buttons.js',
                   config.nodeModulesDir + '/datatables.net-buttons/js/buttons.html5.js',
                   config.nodeModulesDir + '/angular-datatables/dist/plugins/buttons/angular-datatables.buttons.js',
                   config.nodeModulesDir + '/angular-datatables/dist/plugins/colreorder/angular-datatables.colreorder.js',
                   config.nodeModulesDir + '/angular-ui-ace/src/ui-ace.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/ace.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/ext-*.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/theme-*.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/keybinding-*.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/mode-yaml.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/mode-json.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/mode-markdown.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/mode-sql.js',
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/keybinding-emacs.js',
                   config.nodeModulesDir + '/jquery-csv/src/jquery.csv.min.js',
                   config.nodeModulesDir + '/jstree/dist/jstree.js',
                   config.nodeModulesDir + '/moment/moment.js',
                   'third-party/jquery.splitter.js',
                   config.nodeModulesDir + '/sprintf/lib/sprintf.js',
                  ])
        .pipe(gulpNewer(config.distDir + '/third-party.bundle.js'))
        .pipe(gulpConcat('third-party.bundle.js'))
        .pipe(uglify())
        .pipe(gulp.dest(config.distDir));
});


gulp.task('copy-jquery-ui-images', function() {
  return gulp.src([config.nodeModulesDir + '/jquery-ui-dist/images/*.png'])
      .pipe(gulpNewer(config.distDir + '/images'))
      .pipe(gulp.dest(config.distDir + '/images'));
});


gulp.task('copy-fontawesome-fonts', function() {
  return gulp.src([config.nodeModulesDir + '/font-awesome/fonts/fontawesome-webfont.woff2'])
      .pipe(gulp.dest('fonts'));
});

gulp.task('copy-jstree-theme', function() {
    return gulp.src([config.nodeModulesDir + '/jstree-bootstrap-theme/dist/themes/proton/fonts/titillium/*.ttf',
                     config.nodeModulesDir + '/jstree-bootstrap-theme/dist/themes/proton/fonts/titillium/*.woff'])
        .pipe(gulp.dest(config.distDir + '/fonts/titillium/'));
});


gulp.task('copy-third-party-resources', function() {
  return gulp.src([config.nodeModulesDir + '/jstree-bootstrap-theme/dist/themes/proton/*.gif',
                   config.nodeModulesDir + '/jstree-bootstrap-theme/dist/themes/proton/*.png',
                   // This file must be loaded outside the pack.
                   config.nodeModulesDir + '/ace-builds/src-min-noconflict/worker-json.js',
                   config.nodeModulesDir + '/jstree-bootstrap-theme/dist/themes/proton/fonts/titillium/*.ttf',
                   config.nodeModulesDir + '/bootstrap/dist/css/bootstrap.css.map',
                   config.nodeModulesDir + '/bootstrap-sass/assets/fonts/bootstrap/glyphicons-halflings-regular.woff2'])
      .pipe(gulp.dest(config.distDir));
});


gulp.task('compile-third-party-bootstrap-css', function() {
  return gulp.src('less/bootstrap_grr.less')
      .pipe(gulpNewer(config.tempDir + '/grr-bootstrap.css'))
      .pipe(gulpLess({
        paths: [
          config.nodeModulesDir + '/bootstrap-less/bootstrap'
        ]
      }))
      .pipe(gulpConcat('grr-bootstrap.css'))
      .pipe(gulp.dest(config.tempDir));
});


gulp.task('compile-third-party-css', function() {
    return gulp.src([config.nodeModulesDir + '/jstree-bootstrap-theme/dist/themes/proton/style.min.css',
                     config.nodeModulesDir + '/bootstrap/dist/css/bootstrap.css',
                     config.nodeModulesDir + '/angular-ui-bootstrap/dist/ui-bootstrap-csp.css',
                     config.nodeModulesDir + '/ui-select/dist/select.min.css',
                     config.nodeModulesDir + '/font-awesome/css/font-awesome.css',
                     config.nodeModulesDir + '/angular-datatables/dist/css/angular-datatables.css',
                     config.nodeModulesDir + '/datatables/media/css/jquery.dataTables.css',
                     config.nodeModulesDir + '/vis/dist/vis.min.css',
                     config.tempDir + '/grr-bootstrap.css',
                     'third-party/splitter.css'])
        .pipe(gulpNewer(config.distDir + '/third-party.bundle.css'))
        .pipe(gulpConcat('third-party.bundle.css'))
        .pipe(gulp.dest(config.distDir));
});

gulp.task('compile-css', function() {
  return gulp.src([
      'css/_variables.css',
      'angular-components/sidebar/navigator.css',
      'css/base.css',
      'angular-components/artifact/reporting.css',
      'angular-components/client/host-info.css',
      'angular-components/client/shell-cell.css',
      'angular-components/client/virtual-file-system/breadcrumbs.css',
      'angular-components/client/virtual-file-system/file-details.css',
      'angular-components/client/virtual-file-system/file-hex-view.css',
      'angular-components/client/virtual-file-system/file-text-view.css',
      'angular-components/client/virtual-file-system/file-table.css',
      'angular-components/core/global-notifications.css',
      'angular-components/core/wizard-form.css',
      'angular-components/sidebar/client-summary.css',
      'angular-components/user/user-notification-item.css',
      'angular-components/notebook/notebook.css',
  ])
        .pipe(gulpNewer(config.distDir + '/grr-ui.bundle.css'))
        .pipe(gulpConcat('grr-ui.bundle.css'))
        .pipe(gulp.dest(config.distDir));
});


/**
 * GRR tasks.
 */
gulp.task('compile-grr-angular-template-cache', function() {
  return gulp.src('angular-components/**/*.html')
      .pipe(gulpNewer(config.tempDir + '/templates.js'))
      .pipe(gulpAngularTemplateCache({
        module: 'grrUi.templates',
        standalone: true,
        templateHeader:
            'goog.module(\'grrUi.templates.templates.templatesModule\');' +
            'goog.module.declareLegacyNamespace();' +
            'exports = angular.module(\'grrUi.templates\', []);' +
            'angular.module(\'grrUi.templates\').run(["$templateCache", function($templateCache) {'
      }))
      .pipe(gulp.dest(config.tempDir));
});


gulp.task(
    'compile-grr-closure-ui-js',
    function() {
      return gulp
          .src([
            'angular-components/**/*.js',
            '!angular-components/**/*_test.js',
            '!angular-components/empty-templates.js',
            '!angular-components/externs.js',
            config.tempDir + '/templates.js',
          ])
          .pipe(gulpNewer(config.distDir + '/grr-ui.bundle.js'))
          .pipe(gulpPlumber({
            errorHandler: function(err) {
              console.log(err);
              this.emit('end');
              if (!isWatching) {
                process.exit(1);
              }
            }
          }))
        .pipe(sourcemaps.init())
        .pipe(closureCompiler(
          Object.assign({}, closureCompilerFlags, {
            js_output_file: 'grr-ui.bundle.js',
            create_source_map: config.distDir + '/grr-ui.bundle.js.map',
            source_map_location_mapping:
            '|/static/angular-components/',
            angular_pass: true,
            entry_point: 'grrUi.appController',
            externs: [
              'angular-components/externs.js',
            ],
          })))
        .pipe(sourcemaps.write("."))
        .pipe(gulp.dest(config.distDir));
    });

gulp.task('compile-grr-ui-js',
          gulp.series(
              'compile-grr-angular-template-cache',
              'compile-grr-closure-ui-js'));

/**
 * Combined compile tasks.
 */
gulp.task('compile-third-party',
          gulp.series('compile-third-party-js',
                      'compile-third-party-bootstrap-css',
                      'compile-third-party-css',
                      'copy-third-party-resources',
                      'copy-jstree-theme',
                      'copy-fontawesome-fonts',
                      'compile-third-party-bootstrap-css',
                      'compile-third-party-bootstrap-css'));

gulp.task('compile-grr-ui',
          gulp.series('compile-grr-ui-js', 'compile-css'));

gulp.task('compile',
          gulp.series('compile-third-party', 'compile-grr-ui'));

/**
 * "Watch" tasks useful for development.
 */

gulp.task('watch', function() {
  isWatching = true;

  gulp.watch(['javascript/**/*.js', 'angular-components/**/*.js'],
             gulp.series('compile-grr-ui-js'));
  gulp.watch(['css/**/*.css', 'angular-components/**/*.css'],
             gulp.series('compile-css'));
});


gulp.task('clean', function() {
    return del([
        'dist/*',
        '!dist/.keep'
    ]);
});
