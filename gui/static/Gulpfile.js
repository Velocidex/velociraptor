'use strict';

var gulp = require('gulp');
var gulpAngularTemplateCache = require('gulp-angular-templatecache');
var gulpClosureCompiler = require('gulp-closure-compiler');
var gulpConcat = require('gulp-concat');
var gulpInsert = require('gulp-insert');
var gulpLess = require('gulp-less');
var gulpNewer = require('gulp-newer');
var gulpPlumber = require('gulp-plumber');
var gulpSass = require('gulp-sass');
var gulpSourcemaps = require('gulp-sourcemaps');
var karma = require('karma');


var config = {};
config.nodeModulesDir = './node_modules';
config.distDir = 'dist';
config.tempDir = 'tmp';

var isWatching = false;

const closureCompilerPath =
    config.nodeModulesDir + '/google-closure-compiler/compiler.jar';

const closureCompilerFlags = {
  compilation_level: 'WHITESPACE_ONLY',
  dependency_mode: 'STRICT',
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
                   config.nodeModulesDir + '/jquery-migrate/dist/jquery-migrate.js',

                   config.nodeModulesDir + '/google-closure-library/closure/goog/base.js',

                   config.nodeModulesDir + '/bootstrap/dist/js/bootstrap.js',

                   config.nodeModulesDir + '/angular/angular.js',
                   config.nodeModulesDir + '/angular-animate/angular-animate.js',
                   config.nodeModulesDir + '/angular-cookies/angular-cookies.js',
                   config.nodeModulesDir + '/angular-resource/angular-resource.js',

                   config.nodeModulesDir + '/angular-ui-bootstrap/dist/ui-bootstrap-tpls.js',
                   config.nodeModulesDir + '/angular-ui-router/release/angular-ui-router.js',
                   config.nodeModulesDir + '/datatables/media/js/jquery.dataTables.js',
                   config.nodeModulesDir + '/angular-datatables/dist/angular-datatables.js',
                   config.nodeModulesDir + '/angular-ui-ace/src/ui-ace.js',
                   config.nodeModulesDir + '/ace-builds/src-noconflict/ace.js',
                   config.nodeModulesDir + '/ace-builds/src-noconflict/ext-language_tools.js',
                   config.nodeModulesDir + '/ace-builds/src-noconflict/theme-twilight.js',
                   config.nodeModulesDir + '/ace-builds/src-noconflict/mode-yaml.js',
                   config.nodeModulesDir + '/jquery-ui-dist/jquery-ui.js',
                   config.nodeModulesDir + '/jstree/dist/jstree.js',
                   config.nodeModulesDir + '/moment/moment.js',
                   config.nodeModulesDir + '/highlightjs/highlight.pack.js',
                   config.nodeModulesDir + '/marked/lib/marked.js',
                   'third-party/jquery.splitter.js'])
      .pipe(gulpNewer(config.distDir + '/third-party.bundle.js'))
      .pipe(gulpConcat('third-party.bundle.js'))
      .pipe(gulp.dest(config.distDir));
});


gulp.task('copy-jquery-ui-images', function() {
  return gulp.src([config.nodeModulesDir + '/jquery-ui-dist/images/*.png'])
      .pipe(gulpNewer(config.distDir + '/images'))
      .pipe(gulp.dest(config.distDir + '/images'));
});


gulp.task('copy-fontawesome-fonts', function() {
  return gulp.src([config.nodeModulesDir + '/font-awesome/fonts/fontawesome-webfont.*'])
      .pipe(gulp.dest('fonts')); // TODO(user): should be copied to 'dist' folder.
});

gulp.task('copy-third-party-resources', function() {
  return gulp.src([config.nodeModulesDir + '/jstree/dist/themes/default/*.gif',
                   config.nodeModulesDir + '/jstree/dist/themes/default/*.png',
                   config.nodeModulesDir + '/bootstrap-sass/assets/fonts/bootstrap/glyphicons-halflings-regular.*'])
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
  return gulp.src([config.nodeModulesDir + '/jstree/dist/themes/default/style.css',
                   config.nodeModulesDir + '/bootstrap/dist/css/bootstrap.css',
                   config.nodeModulesDir + '/angular-ui-bootstrap/dist/ui-bootstrap-csp.css',
                   config.nodeModulesDir + '/font-awesome/css/font-awesome.css',
                   config.nodeModulesDir + '/jquery-ui-dist/jquery-ui.css',
                   config.nodeModulesDir + '/angular-datatables/dist/css/angular-datatables.css',
                   config.nodeModulesDir + '/datatables/media/css/jquery.dataTables.css',
                   config.nodeModulesDir + '/jquery-ui-dist/jquery-ui.theme.css',
                   config.nodeModulesDir + '/highlightjs/styles/atom-one-light.css',
                   config.tempDir + '/grr-bootstrap.css',
                   'third-party/splitter.css'])
      .pipe(gulpNewer(config.distDir + '/third-party.bundle.css'))
      .pipe(gulpConcat('third-party.bundle.css'))
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
          .pipe(gulpClosureCompiler({
            compilerPath: closureCompilerPath,
            fileName: 'grr-ui.bundle.js',
            compilerFlags: Object.assign({}, closureCompilerFlags, {
              angular_pass: true,
              entry_point: 'grrUi.appController',
              externs: [
                'angular-components/externs.js',
              ],
              create_source_map: config.distDir + '/grr-ui.bundle.js.map',
              source_map_location_mapping:
                  'angular-components/|/static/angular-components/',
            }),
          }))
          .pipe(gulpInsert.append('//# sourceMappingURL=grr-ui.bundle.js.map'))
          .pipe(gulp.dest(config.distDir));
    });


gulp.task('compile-grr-ui-tests', function() {
  return gulp.src(['angular-components/**/*_test.js'])
      .pipe(gulpNewer(config.distDir + '/grr-ui-test.bundle.js'))
      .pipe(gulpPlumber({
        errorHandler: function(err) {
          console.log(err);
          this.emit('end');
          if (!isWatching) {
            process.exit(1);
          }
        }
      }))
      .pipe(gulpClosureCompiler({
        compilerPath: closureCompilerPath,
        fileName: 'grr-ui-test.bundle.js',
        compilerFlags: Object.assign({}, closureCompilerFlags, {
          angular_pass: true,
          dependency_mode: 'NONE',
          externs: [
            'angular-components/externs.js',
          ],
          create_source_map: config.distDir + '/grr-ui-test.bundle.js.map',
          source_map_location_mapping:
              'angular-components/|/static/angular-components/',
        }),
      }))
      .pipe(gulpInsert.append('//# sourceMappingURL=grr-ui-test.bundle.js.map'))
      .pipe(gulp.dest(config.distDir));
});


gulp.task('compile-grr-ui-js',
          gulp.series(
              'compile-grr-angular-template-cache',
              'compile-grr-closure-ui-js'));

gulp.task('compile-grr-ui-css', function() {
  return gulp.src(['css/base.scss'])
      .pipe(gulpNewer(config.distDir + '/grr-ui.bundle.css'))
      .pipe(gulpPlumber({
        errorHandler: function(err) {
          console.log(err);
          this.emit('end');

          if (!isWatching) {
            process.exit(1);
          }
        }
      }))
      .pipe(gulpSass({
          includePaths: [
            '../../../../../'
          ]
      }).on('error', gulpSass.logError))
      .pipe(gulpConcat('grr-ui.bundle.css'))
      .pipe(gulp.dest(config.distDir));
});


/**
 * Combined compile tasks.
 */
gulp.task('compile-third-party',
          gulp.series('compile-third-party-js',
                      'compile-third-party-bootstrap-css',
                      'compile-third-party-css',
                      'copy-third-party-resources',
                      'copy-jquery-ui-images',
                      'copy-fontawesome-fonts',
                      'compile-third-party-bootstrap-css',
                      'compile-third-party-bootstrap-css'));

gulp.task('compile-grr-ui',
          gulp.series('compile-grr-ui-js', 'compile-grr-ui-css'));

gulp.task('compile',
          gulp.series('compile-third-party', 'compile-grr-ui'));

/**
 * "Watch" tasks useful for development.
 */

gulp.task('watch', function() {
  isWatching = true;

  gulp.watch(['javascript/**/*.js', 'angular-components/**/*.js'],
             gulp.series('compile-grr-ui-js'));
  gulp.watch(['css/**/*.css', 'css/**/*.scss', 'angular-components/**/*.scss'],
             gulp.series('compile-grr-ui-css'));
});


gulp.task('test', gulp.series('compile', function(done) {
  let config = {
    configFile: __dirname + '/karma.conf.js',
    browsers: ['ChromeHeadlessNoSandbox'],
    singleRun: true,
  };

  new karma.Server(config, done).start();
}));


gulp.task('test-debug', gulp.series('compile', function(done) {
  let config = {
    configFile: __dirname + '/karma.conf.js',
    browsers: ['Chrome'],
  };

  new karma.Server(config, done).start();
}));
