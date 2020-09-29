import React from 'react';
import PropTypes from 'prop-types';

export default class FlowOverview extends React.Component {
    static propTypes = {
        flow: PropTypes.object,
    };

    render() {
        if (!this.props.flow || !this.props.flow.session_id) {
            return <div>Loading ...</div>;
        }

        return (
            <>
              <div>
                <grr-force-refresh refresh-trigger="flow">
                  <div className="card-deck">
                    <div className="card panel">
                      <h5 className="card-header">Overview</h5>
                      <div className="card-body">
                        <dl className="dl-horizontal dl-flow">
                          <dt>Artifact Names</dt>
                          <dd>
                            <div ng-repeat="item in flow.context.request.artifacts || []">
                              item
                            </div>
                          </dd>

                          <dt>Flow ID</dt>
                          <dd>  flow.context.session_id </dd>

                          <dt>Creator</dt>
                          <dd> flow.context.request.creator </dd>

                          <dt>Create Time</dt>
                          <dd>
                            <grr-timestamp value="flow.context.create_time">
                            </grr-timestamp>
                          </dd>

                          <dt>Start Time</dt>
                          <dd>
                            <grr-timestamp value="flow.context.start_time">
                            </grr-timestamp>
                          </dd>

                          <dt>Last Active</dt>
                          <dd><grr-timestamp value="flow.context.active_time"></grr-timestamp></dd>

                          <dt>Duration</dt>
                          <dd>flow.context.execution_duration/1000000000 | number:2
                            <span ng-if="flow.context.execution_duration">Seconds</span>
                            <span ng-if="!flow.context.execution_duration">Running....</span>
                          </dd>

                          <dt>State</dt>
                          <dd>flow.context.state</dd>

                          <div ng-if="::flow.context.state == 'ERROR'">
                            <dt>Error</dt>
                            <dd>flow.context.status</dd>
                          </div>

                          <dt>Ops/Sec</dt>
                          <dd>::flow.context.request.ops_per_second || 'Unlimited' </dd>
                          <dt>Timeout</dt>
                          <dd>::flow.context.request.timeout || 'Unlimited' </dd>
                          <dt>Max Rows</dt>
                          <dd>::flow.context.request.max_rows || 'Unlimited' </dd>
                          <dt>Max Mb</dt>
                          <dd> ( (flow.context.request.max_upload_bytes || 0) / 1024 / 1024) || 'Unlimited' </dd>

                          <br />
                        </dl>

                        <h5> Parameters </h5>
                        <dl className="dl-horizontal dl-flow">
                          <div ng-repeat="item in ::flow.context.request.parameters.env">
                            <dt>item.key</dt>
                            <dd>item.value</dd>
                          </div>
                        </dl>

                      </div>
                    </div>
                    <div className="card panel">
                      <h5 className="card-header">Results</h5>
                      <div className="card-body">
                        <dl className="dl-horizontal dl-flow">
                          <dt>Artifacts with Results</dt>
                          <dd>
                            <div ng-repeat="item in flow.context.artifacts_with_results || []">
                              item
                            </div>
                          </dd>

                          <dt>Total Rows</dt>
                          <dd>
                            flow.context.total_collected_rows || 0
                          </dd>

                          <dt>Uploaded Bytes</dt>
                          <dd>
                            flow.context.total_uploaded_bytes || 0  /  flow.context.total_expected_uploaded_bytes || 0
                          </dd>

                          <dt>Files uploaded</dt>
                          <dd>  flow.context.uploaded_files.length || flow.context.total_uploaded_files || 0 }} </dd>

                          <dt>Download Results</dt>
                          <dd>
                            <div className="input-group-prepend">
                              <button className="btn btn-default dropdown-toggle"
                                      type="button"
                                      data-toggle="dropdown" aria-expanded="false"
                                      ng-disabled="!controller.uiTraits.Permissions.prepare_results"
                              >
                                <i className="fa fa-archive"></i>
                              </button>
                              <div className="dropdown-menu">
                                <button className="dropdown-item"
                                        ng-disabled="!controller.uiTraits.Permissions.prepare_results"
                                        ng-click="controller.prepareDownload()">
                                  Prepare Download
                                </button>
                                <button className="dropdown-item"
                                        ng-disabled="!controller.uiTraits.Permissions.prepare_results"
                                        ng-click="controller.prepareDownload('report')">
                                  Prepare Collection Report
                                </button>
                              </div>
                            </div>
                          </dd>

                          <dt>Available Downloads</dt>
                          <dd>
                            <table className="table table-stiped table-condensed table-hover table-bordered">
                              <tbody>
                                <tr ng-repeat="item in flow.available_downloads.files">
                                  <td>
                                    <a href="item.path}}"
                                       target="_blank"
                                       ng-if="item.complete">item.name}}</a>
                                    <div ng-if="!item.complete">item.name}}</div>
                                  </td>
                                  <td>item.size}} Bytes</td>
                                  <td>item.date}}</td>
                                </tr>
                              </tbody>
                            </table>
                          </dd>
                        </dl>
                      </div>
                    </div>
                  </div>
                  <div ng-if="::flow.internal_error">
                    <br/>
                    <dt className="alert-danger danger">Error while Opening</dt>
                    <dd> ::flow.internal_error </dd>
                  </div>

                </grr-force-refresh>
              </div>

            </>
        );
    }
};
