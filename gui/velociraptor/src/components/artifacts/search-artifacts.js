import React from 'react';
import PropTypes from 'prop-types';

export default class SearchArtifacts extends React.Component {
    static propTypes = {

    };

    render() {
        return (
            <>
              <div id="artifact_search">
                <div className="card">
                  <div id="headingOne">
                    <button className="btn btn-default btn-block"
                            data-toggle="collapse"
                            data-target="#collapseOne"
                            aria-expanded="true"
                            aria-controls="collapseOne">
                      Search for artifacts
                    </button>
                  </div>

                  <div id="collapseOne"
                       className="collapse show"
                       aria-labelledby="headingOne"
                       data-parent="#artifact_search">
                    <div className="card-body">
                      <div className="col-md-5">
                        <div className="input-group">
                          <input name="Search" className="form-control"
                                 type="text" placeholder="Search"
                                 autocomplete="off"
                                 focus-me="controller.search_focus"
                                 ng-model="controller.search"></input>
                        </div>

                        <div XXXstyle="height: 150px; overflow-y: auto; overflow-x: hidden; border: 1px solid #dddddd; border-top: none">

                          <table name="Artifacts"
                                 className="table table-condensed table-hover table-striped">
                            <colgroup>
                              <col width="100%"></col>
                            </colgroup>

                            <tbody>
                              <tr ng-if="!controller.descriptorsError && controller.descriptors === undefined">
                                <td>
                                  Search for an artifact by typing above.
                                </td>
                              </tr>
                              <tr ng-if="controller.descriptorsError" className="alert-danger danger">
                                <td>
                                  <strong>Can't fetch artifacts list:</strong><br/>
                                  <span className="preserve-linebreaks"> controller.descriptorsError
                                  </span>
                                </td>
                              </tr>
                              <tr ng-repeat="descriptor in controller.matchingDescriptors"
                                  ng-className="'row-selected': descriptor.name == controller.selectedName" >
                                <td style={{cursor: "pointer", border: "none"}}>
                                  <a ng-dblclick="controller.add(descriptor.name, $event)"
                                     href="#"
                                     ng-click="controller.selectArtifact(descriptor.name, $event)"
                                     style={{display: "block"}}
                                     className="full-width-height artifact-links">
                                    ::descriptor.name
                                  </a>
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>

                        <div className="full-width">
                          <div className="pull-left" style={{"padding-top": "1.5em"}}>
                            <p>Selected Artifacts:</p>
                          </div>
                          <div className="pull-right">
                            <button className="btn btn-default btn-sm form-add" style={{
                                "margin-top": "0.5em"}} name="Add"
                                    id="add_artifact"
                                    ng-disabled="!controller.selectedName"
                                    ng-click="controller.add(controller.selectedName)">
                              Add
                            </button>
                          </div>
                          <div className="clearfix"></div>
                        </div>

                        <div style={{"margin-top": "0em", height: "150px", "overflow-y": "auto", "overflow-x": "hidden", border: "1px solid #dddddd"}}>

                          <table name="SelectedArtifacts" className="table table-condensed table-hover table-striped">
                            <colgroup>
                              <col width="100%"></col>
                            </colgroup>

                            <tbody>
                              <tr ng-repeat="name in names"
                                  ng-className="row-selected': name == controller.selectedName">
                                <td style={{cursor: "pointer", border: "none"}}
                                    ng-dblclick="controller.remove(name)"
                                    ng-click="controller.selectArtifact(name)"
                                    ng-className="row-selected': name == controller.selectedName">
                                  <div className="full-width-height">
                                    <strong> ::name </strong>
                                  </div>
                                </td>
                              </tr>
                              <tr ng-if="names.length == 0">
                                <td>
                                  <em>Use "Add" button or double-click to add artifacts to the list.</em>
                                </td>
                              </tr>
                            </tbody>
                          </table>
                        </div>

                        <div className="full-width" >
                          <button className="btn btn-default btn-sm form-add" name="Add"
                                  ng-click="controller.clear()">
                            Clear
                          </button>

                          <div className="pull-right">
                            <button className="btn btn-default btn-sm form-add" name="Add"
                                    ng-disabled="!controller.selectedName"
                                    ng-click="controller.remove(controller.selectedName)">
                              Remove
                            </button>
                          </div>
                        </div>
                      </div>

                      <div name="ArtifactInfo" className="col-md-7 artifact-description"
                           grr-force-refresh refresh-trigger="controller.selectedName">
                        <grr-report-viewer ng-if="controller.selectedName"
                                           params="controller.reportParams">
                        </grr-report-viewer>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              <div id="configure_parameters">
                <div className="card">
                  <div  id="headingTwo">
                    <button className="btn btn-default btn-block"
                            data-toggle="collapse"
                            data-target="#collapseTwo"
                            aria-expanded="false"
                            focus-me="controller.focus_parameters"
                            aria-controls="collapseTwo">
                      Configure parameters
                    </button>
                  </div>
                  <div id="collapseTwo" className="collapse show" aria-labelledby="headingTwo" data-parent="#configure_parameters">
                    <div className="card-body">
                      <table className="table table-condensed">
                        <tr ng-repeat="(k, v) in params "
                            ng-if="controller.param_types[k] != 'hidden'">
                          <div data-placement="left"
                               data-toggle="tooltip"
                               title=" controller.param_descriptions[k] ">
                            <td>
                              k
                            </td>
                            <td>
                              <grr-form type="controller.param_types[k]"
                                        value="params"
                                        info="controller.param_info[k]"
                                        description="controller.param_descriptions[k]"
                                        field="k"></grr-form>
                            </td>
                          </div>
                        </tr>
                      </table>
                    </div>
                  </div>
                </div>
                <div id="tools">
                  <div className="card">
                    <div id="headingTools">
                      <button className="btn btn-default btn-block"
                              data-toggle="collapse"
                              data-target="#collapseTools"
                              aria-expanded="true"
                              aria-controls="collapseTools">
                        Tools Used
                      </button>
                    </div>

                    <div id="collapseTools"
                         className="collapse show"
                         aria-labelledby="headingTools"
                         data-parent="#tools">
                      <div className="card-body">
                        <dl className="dl-horizontal dl-flow"
                            ng-repeat="(tool, definition) in tools track by tool"
                        >
                          <dt></dt>
                          <dd><grr-tool-viewer name=" tool "></grr-tool-viewer></dd>
                        </dl>
                      </div>
                    </div>
                  </div>
                </div>
              </div>

            </>
        );
    }
};
