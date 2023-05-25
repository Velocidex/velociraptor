import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import Modal from 'react-bootstrap/Modal';
import {CancelToken} from 'axios';
import StepWizard from 'react-step-wizard';
import T from '../i8n/i8n.jsx';

import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import { HotKeys, ObserveKeys } from "react-hotkeys";
import DateTimePicker from 'react-datetime-picker';
import EstimateHunt from './estimate.jsx';
import LabelForm from '../utils/labels.jsx';
import api from '../core/api-service.jsx';
import NewCollectionConfigParameters from '../flows/new-collections-parameters.jsx';
import { OrgSelectorForm } from './orgs.jsx';

import {
    NewCollectionSelectArtifacts,
    NewCollectionResources,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
} from '../flows/new-collection.jsx';

import UserConfig from '../core/user.jsx';

// The hunt wizard is built upon the new collection wizard with some extra steps.
class HuntPaginator extends PaginationBuilder {
    PaginationSteps = ["Configure Hunt",
                       "Select Artifacts", "Configure Parameters",
                       "Specify Resources", "Review", "Launch"];
}

class NewHuntConfigureHunt extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
        parameters: PropTypes.object,
        paginator: PropTypes.object,
        setHuntParameters: PropTypes.func.isRequired,
    }

    setParam = (k,v) => {
        this.props.parameters[k] = v;
        this.props.setHuntParameters(this.props.parameters);
    }

    render() {
        let is_admin = this.context.traits &&
            this.context.traits.Permissions &&
            this.context.traits.Permissions.server_admin;

        let is_start_hunt = this.context.traits &&
            this.context.traits.Permissions &&
            this.context.traits.Permissions.start_hunt;

        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{T("New Hunt - Configure Hunt")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Description")}</Form.Label>
                    <Col sm="8">
                      <Form.Control as="textarea" rows={3}
                                    id="hunt-description-text"
                                    placeholder={T("Hunt description")}
                                    spellCheck="false"
                                    value={this.props.parameters.description}
                                    onChange={e => this.setParam("description", e.target.value)}
                      />
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Expiry")}</Form.Label>
                    <Col sm="8">
                      <DateTimePicker value={this.props.parameters.expires}
                                      onChange={(value) => this.setParam("expires", value)}
                      />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Include Condition")}</Form.Label>
                    <Col sm="8">
                        <Form.Control as="select"
                          value={this.props.parameters.include_condition}
                          onChange={(e) => this.setParam(
                              "include_condition", e.currentTarget.value)}
                          >
                          <option label={T("Run everywhere")} value="">{T("Run everywhere")}</option>
                          <option label={T("Match by label")} value="labels">{T("Match by label")}</option>
                          <option label={T("Operating System")} value="os">{T("Operating System")}</option>
                        </Form.Control>
                    </Col>
                  </Form.Group>
                  { this.props.parameters.include_condition === "os" &&
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">{T("Operating System Included")}</Form.Label>
                      <Col sm="8">
                        <Form.Control as="select"
                                      value={this.props.parameters.include_os}
                                      onChange={(e) => this.setParam(
                                          "include_os", e.currentTarget.value)}
                        >
                          <option label={T("ALL")} value="ALL">{T("ALL")}</option>
                          <option label="Windows" value="WINDOWS">Windows</option>
                          <option label="Linux" value="LINUX">LINUX</option>
                          <option label="MacOS" value="OSX">OSX</option>
                        </Form.Control>
                      </Col>
                    </Form.Group>
                  }

                  { this.props.parameters.include_condition === "labels" &&
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">{T("Include Labels")}</Form.Label>
                      <Col sm="8">
                        <LabelForm
                          value={this.props.parameters.include_labels}
                          onChange={(value) => this.setParam("include_labels", value)}
                        />
                      </Col>
                    </Form.Group>
                  }

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">{T("Exclude Condition")}</Form.Label>
                    <Col sm="8">
                        <Form.Control as="select"
                          value={this.props.parameters.exclude_condition}
                          onChange={(e) => this.setParam(
                              "exclude_condition", e.currentTarget.value)}
                          >
                          <option label={T("Run everywhere")} value="">{T("Run everywhere")}</option>
                          <option label={T("Match by label")} value="labels">{T("Match by label")}</option>
                        </Form.Control>
                    </Col>
                  </Form.Group>

                  { this.props.parameters.exclude_condition === "labels" &&
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">{T("Exclude Labels")}</Form.Label>
                      <Col sm="8">
                        <LabelForm
                          value={this.props.parameters.excluded_labels}
                          onChange={(value) => this.setParam("excluded_labels", value)}
                        />
                      </Col>
                    </Form.Group>
                  }

                  { is_admin &&
                      <OrgSelectorForm
                        value={this.props.parameters.org_ids}
                        onChange={(value) => this.setParam("org_ids", value)} />
                  }
                  { is_start_hunt &&
                      <Form.Group as={Row}>
                        <Form.Label column sm="3">{T("Hunt State")}</Form.Label>
                        <Col sm="8">
                          <Form.Check
                            value={this.props.parameters.force_start}
                            label={T("Start Hunt Immediately")}
                            onChange={e=>this.setParam(
                                "force_start", !this.props.parameters.force_start)}
                          />
                        </Col>
                      </Form.Group>
                  }
                  <EstimateHunt
                    params={this.props.parameters}/>

                </Form>
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    step_name: "Configure Hunt",
                }) }
              </Modal.Footer>
            </>
        );
    }
}


export default class NewHuntWizard extends React.Component {
    static contextType = UserConfig;

    static propTypes = {
        baseHunt: PropTypes.object,
        onResolve: PropTypes.func,
        onCancel: PropTypes.func,
    }

    state = {
        // A list of artifact descriptors we have selected so far.
        artifacts: [],

        // A key/value mapping of edited parameters by the user.
        parameters: {},

        resources: {},

        hunt_parameters: {
            include_condition: "",
            include_labels: [],
            include_os: "ALL", // Default selector
            exclude_condition: "",
            excluded_labels: [],
        },
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        let state = this.setStateFromBase(this.props.baseHunt || {});
        this.setState(state);

        // A bit hacky but whatevs...
        const el = document.getElementById("hunt-description-text");
        if (el) {
            setTimeout(()=>el.focus(), 100);
        };
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        this.setState({parameters: params});
    }

    setResources = (resources) => {
        let new_resources = Object.assign(this.state.resources, resources);
        this.setState({resources: new_resources});
    }

    // Let our caller know the artifact request we created.
    launch = () => {
        this.props.onResolve(this.prepareRequest());
    }

    setStateFromBase = (hunt) => {
        let request = hunt && hunt.start_request;
        let expiry = new Date();

        let hunt_expiry_hours = this.context.traits && this.context.traits.customizations &&
            this.context.traits.customizations.hunt_expiry_hours;
        if (!hunt_expiry_hours) {hunt_expiry_hours = 7 * 24;};

        expiry.setTime(expiry.getTime() + hunt_expiry_hours * 60 * 60 * 1000);

        if (request) {
            let state = {
                artifacts: [],
                parameters: {},
                resources: {
                    max_rows: request.max_rows,
                    max_mbytes: parseInt(request.max_upload_bytes / 1024/1024),
                    timeout: request.timeout,
                    ops_per_second: request.ops_per_second,
                },
                hunt_parameters: this.state.hunt_parameters,
            };

            let labels = hunt.condition && hunt.condition.labels &&
                hunt.condition.labels.label;
            if (!_.isEmpty(labels)) {
                state.hunt_parameters.include_labels = labels;
                state.hunt_parameters.include_condition = "labels";
            }

            let os = hunt.condition && hunt.condition.os &&
                hunt.condition.os.os;
            if (!_.isEmpty(os)) {
                state.hunt_parameters.include_os = os;
                state.hunt_parameters.include_condition = "os";
            }

            let excluded = hunt.condition && hunt.condition.excluded_labels &&
                hunt.condition.excluded_labels.labels;
            if (!_.isEmpty(excluded)) {
                state.hunt_parameters.excluded_labels = excluded;
            }
            state.hunt_parameters.description = hunt.hunt_description;
            state.hunt_parameters.expires = expiry;
            state.hunt_parameters.org_ids = hunt.org_ids || [];

            // Resolve the artifacts from the request into a list of descriptors.
            api.post("v1/GetArtifacts", {
                names: request.artifacts,
            }, this.source.token).then(response=>{
                if (response && response.data &&
                    response.data.items && response.data.items.length) {

                    this.setState({artifacts: [...response.data.items]});

                    // New style request.
                    if (!_.isEmpty(request.specs)) {
                        let parameters = {};
                        _.each(request.specs, spec=>{
                            let artifact_parameters = {};
                            _.each(spec.parameters.env, param=>{
                                artifact_parameters[param.key] = param.value;
                            });
                            parameters[spec.artifact] = artifact_parameters;
                        });

                        this.setState({parameters: parameters});
                    } else {
                        let parameters = {};
                        if (!_.isEmpty(request.parameters)) {
                            _.each(request.artifacts, name=>{
                                _.each(request.parameters.env, param=>{
                                    if(_.isUndefined(parameters[name])) {
                                        parameters[name] = {};
                                    };
                                    parameters[name][param.key] = param.value;
                                });
                            });
                        }

                        this.setState({parameters: parameters});
                    };
                }});


            return state;
        };

        let state = this.state;
        state.hunt_parameters.expires = expiry;
        return state;
    }

    prepareRequest = () => {
        let specs = [];
        let artifacts = [];
        _.each(this.state.artifacts, (item) => {
            let spec = {
                artifact: item.name,
                parameters: {env: []},
            };

            _.each(this.state.parameters[item.name], (v, k) => {
                spec.parameters.env.push({key: k, value: v});
            });
            specs.push(spec);
            artifacts.push(item.name);
        });

        let request = {
            artifacts: artifacts,
            specs: specs,
        };

        if (this.state.resources.progress_timeout) {
            request.progress_timeout = this.state.resources.progress_timeout;
        }

        if (this.state.resources.cpu_limit) {
            request.cpu_limit = this.state.resources.cpu_limit;
        }

        if (this.state.resources.iops_limit) {
            request.iops_limit = this.state.resources.iops_limit;
        }

        if (this.state.resources.timeout) {
            request.timeout = this.state.resources.timeout;
        }

        if (this.state.resources.max_rows) {
            request.max_rows = this.state.resources.max_rows;
        }

        if (this.state.resources.max_mbytes) {
            request.max_upload_bytes = this.state.resources.max_mbytes * 1024 * 1024;
        }

        let result = {
            start_request: request,
            condition: {},
        };

        let hunt_parameters = this.state.hunt_parameters;
        if (hunt_parameters.expires) {
            result.expires = hunt_parameters.expires.getTime() * 1000;
        }

        if (hunt_parameters.include_condition === "labels") {
            result.condition.labels = {"label": hunt_parameters.include_labels};
        }
        if (hunt_parameters.include_condition === "os" &&
            hunt_parameters.include_os !== "ALL") {
            result.condition.os = {"os": hunt_parameters.include_os};
        }

        if (hunt_parameters.exclude_condition === "labels") {
            result.condition.excluded_labels = {label: hunt_parameters.excluded_labels};
        }

        if (hunt_parameters.description) {
            result.hunt_description = hunt_parameters.description;
        }

        if (hunt_parameters.org_ids) {
            result.org_ids = hunt_parameters.org_ids;
        }

        if (hunt_parameters.force_start) {
            result.state = 2;
        }

        return result;
    }

    gotoStep = (step) => {
        return e=>{
            this.step.goToStep(step);
            e.preventDefault();
        };
    };

    gotoNextStep = () => {
        return e=>{
            this.step.nextStep();
            e.preventDefault();
        };
    }

    gotoPrevStep = () => {
        return e=>{
            this.step.previousStep();
            e.preventDefault();
        };
    }

    render() {
        let keymap = {
            GOTO_LAUNCH: "ctrl+l",
            NEXT_STEP: "ctrl+right",
            PREV_STEP: "ctrl+left",
        };
        let handlers={
            GOTO_LAUNCH: this.gotoStep(5),
            NEXT_STEP: this.gotoNextStep(),
            PREV_STEP: this.gotoPrevStep(),
        };
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <HotKeys keyMap={keymap} handlers={handlers}><ObserveKeys>
              <StepWizard ref={n=>this.step=n}>
                <NewHuntConfigureHunt
                  parameters={this.state.hunt_parameters}
                  paginator={new HuntPaginator(
                      "Configure Hunt",
                      "Create Hunt: Configure hunt",
                      (isFocused, step) => {
                          // Focus on this tab when no artifacts are
                          // selected. Only allow artifact selection
                          // pane to be on.  Note that we are ok with
                          // an empty hunt description.
                          return _.isEmpty(this.state.artifacts) && step !== "Select Artifacts";
                      })}
                  setHuntParameters={x => this.setState({hunt_parameters: x})}
                />

                <NewCollectionSelectArtifacts
                  paginator={new HuntPaginator(
                      "Select Artifacts",
                      "Create Hunt: Select artifacts to collect",
                      (isFocused, step) => {
                          // Focus on this step if the component wants
                          // focus, but still allow the configure
                          // wizard step to be visible.
                          return isFocused && step !== "Configure Hunt";
                      })}
                  artifacts={this.state.artifacts}
                  artifactType="CLIENT"
                  setParameters={this.setParameters}
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  parameters={this.state.parameters}
                  setParameters={this.setParameters}
                  artifacts={this.state.artifacts}
                  setArtifacts={this.setArtifacts}
                  paginator={new HuntPaginator("Configure Parameters",
                                               "Create Hunt: Configure artifact parameters")}
                />

                <NewCollectionResources
                  resources={this.state.resources}
                  paginator={new HuntPaginator("Specify Resources",
                                               "Create Hunt: Specify resource limits")}

                  setResources={this.setResources} />

                <NewCollectionRequest
                  paginator={new HuntPaginator("Review",
                                              "Create Hunt: Review request")}
                  request={this.prepareRequest()} />

                <NewCollectionLaunch
                  artifacts={this.state.artifacts}
                  paginator={new PaginationBuilder(
                      "Launch",
                      "Create Hunt: Launch hunt")}
                  launch={this.launch} />

              </StepWizard>
              </ObserveKeys></HotKeys>
            </Modal>
        );
    }
}
