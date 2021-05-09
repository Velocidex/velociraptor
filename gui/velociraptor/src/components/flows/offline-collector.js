import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Modal from 'react-bootstrap/Modal';
import StepWizard from 'react-step-wizard';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import ToolViewer from "../tools/tool-viewer.js";
import { HotKeys, ObserveKeys } from "react-hotkeys";
import ValidatedInteger from '../forms/validated_int.js';
import api from '../core/api-service.js';

import {
    NewCollectionSelectArtifacts,
    NewCollectionConfigParameters,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
} from './new-collection.js';


// The offline collection wizard is built upon the new collection
// wizard with some extra steps.
class OfflinePaginator extends PaginationBuilder {
    PaginationSteps = ["Select Artifacts", "Configure Parameters",
                       "Configure Collection",
                       "Review", "Launch"];
}


class OfflineCollectorParameters  extends React.Component {
    static propTypes = {
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
    }

    state = {
        template_artifacts: [],
    }

    componentDidMount = () => {
        api.get("v1/GetArtifacts", {report_type: "html"}).then((response) => {
            let template_artifacts = [];
            for(var i = 0; i<response.data.items.length; i++) {
                var item = response.data.items[i];
                template_artifacts.push(item["name"]);
            };

            this.setState({template_artifacts: template_artifacts});
        });
    }

    render() {
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>Create Offline collector:  Configure Collector</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Target Operating System</Form.Label>
                    <Col sm="8">
                      <Form.Control as="select"
                                    value={this.props.parameters.target_os}
                                    onChange={(e) => {
                                        this.props.parameters.target_os = e.currentTarget.value;
                                        this.props.setParameters(this.props.parameters);
                                    }}
                      >
                        <option value="Windows">Windows</option>
                        <option value="Windows_x86">Windows_x86</option>
                        <option value="Linux">Linux</option>
                        <option value="MacOS">Mac OS</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Password</Form.Label>
                    <Col sm="8">
                      <Form.Control as="input"
                                    placeholder="Password"
                                    value={this.props.parameters.password}
                                    onChange={e => {
                                        this.props.parameters.password = e.target.value;
                                        this.props.setParameters(this.props.parameters);
                                    }} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Report Template</Form.Label>
                    <Col sm="8">
                      <Form.Control as="select"
                                    value={this.props.parameters.template}
                                    onChange={(e) => {
                                        this.props.parameters.template = e.currentTarget.value;
                                        this.props.setParameters(this.props.parameters);
                                    }}
                      >
                        <option value="">No Report</option>
                        { _.map(this.state.template_artifacts, (item, i)=>{
                            return <option key={i} value={item}>{item}</option>;
                        })}
                      </Form.Control>
                    </Col>
                  </Form.Group>


                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Collection Type</Form.Label>
                    <Col sm="8">
                      <Form.Control as="select"
                                    value={this.props.parameters.target}
                                    onChange={(e) => {
                                        this.props.parameters.target = e.currentTarget.value;
                                        this.props.setParameters(this.props.parameters);
                                    }}
                      >
                        <option value="ZIP">Zip Archive</option>
                        <option value="GCS">Google Cloud Bucket</option>
                        <option value="S3">AWS Bucket</option>
                        <option value="SFTP">SFTP Upload</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>

                  { this.props.parameters.target === "GCS" && <>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">GCS Bucket</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Bucket name"
                                      value={this.props.parameters.target_args.bucket}
                                      onChange={e => {
                                          this.props.parameters.target_args.bucket = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                      <Form.Label column sm="3">GCS Key Blob</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="GCS Blob"
                                      value={this.props.parameters.target_args.GCSKey}
                                      onChange={e => {
                                          this.props.parameters.target_args.GCSKey = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group> </>

                  }
                  { this.props.parameters.target === "S3" && <>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">S3 Bucket </Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Bucket name"
                                      value={this.props.parameters.target_args.bucket}
                                      onChange={e => {
                                          this.props.parameters.target_args.bucket = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Credentials Key</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Credentials Key"
                                      value={this.props.parameters.target_args.credentialsKey}
                                      onChange={e => {
                                          this.props.parameters.target_args.credentialsKey = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Credentials Secret</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Credentials Secret"
                                      value={this.props.parameters.target_args.credentialsSecret}
                                      onChange={e => {
                                          this.props.parameters.target_args.credentialsSecret = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Region</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Region"
                                      value={this.props.parameters.target_args.region}
                                      onChange={e => {
                                          this.props.parameters.target_args.region = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Endpoint</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Endpoint (blank for AWS)"
                                      value={this.props.parameters.target_args.endpoint}
                                      onChange={e => {
                                          this.props.parameters.target_args.endpoint = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Server Side Encryption</Form.Label>
                      <Col sm="8">
                        <Form.Control as="select"
                                        value={this.props.parameters.target_args.serverSideEncryption}
                                        onChange={(e) => {
                                            this.props.parameters.target_args.serverSideEncryption = e.target.value;
                                            this.props.setParameters(this.props.parameters);
                                        }}
                        >
                            <option value="">None</option>
                            <option value="aws:kms">aws:kms</option>
                            <option value="AES256">AES256</option>
                        </Form.Control>
                      </Col>
                    </Form.Group>
                    <Form.Group as={Row}>
                        <Form.Label column sm="3">Skip Cert Verification</Form.Label>
                        <Col sm="8">
                        <Form.Control as="select"
                                        value={this.props.parameters.target_args.noverifycert}
                                        onChange={(e) => {
                                            this.props.parameters.target_args.noverifycert = e.target.value;
                                            this.props.setParameters(this.props.parameters);
                                        }}
                        >
                            <option value="N">N</option>
                            <option value="Y">Y</option>
                        </Form.Control>
                        </Col>
                    </Form.Group>
                    </>
                  }

                  { this.props.parameters.target === "SFTP" && <>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Upload Path</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Upload Path"
                                      value={this.props.parameters.target_args.path}
                                      onChange={e => {
                                          this.props.parameters.target_args.path = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Private Key</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Private Key"
                                      value={this.props.parameters.target_args.privatekey}
                                      onChange={e => {
                                          this.props.parameters.target_args.privatekey = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                        <Form.Label column sm="3">User</Form.Label>
                        <Col sm="8">
                            <Form.Control as="textarea" rows={3}
                                    placeholder="User"
                                    value={this.props.parameters.target_args.user}
                                    onChange={e => {
                                        this.props.parameters.target_args.user = e.target.value;
                                        this.props.setParameters(this.props.parameters);
                                    }}
                        />
                    </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                        <Form.Label column sm="3">Endpoint</Form.Label>
                        <Col sm="8">
                            <Form.Control as="textarea" rows={3}
                                    placeholder="Endpoint"
                                    value={this.props.parameters.target_args.endpoint}
                                    onChange={e => {
                                        this.props.parameters.target_args.endpoint = e.target.value;
                                        this.props.setParameters(this.props.parameters);
                                    }}
                            />
                        </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Host Key</Form.Label>
                        <Col sm="8">
                          <Form.Control as="textarea" rows={3}
                            placeholder="Leave Blank to disable host key checking"
                            value={this.props.parameters.target_args.hostkey}
                                onChange={(e) => {
                                    this.props.parameters.target_args.hostkey = e.target.value;
                                    this.props.setParameters(this.props.parameters);
                                }}
                            />
                    </Col>
                    </Form.Group>
                    </>
                  }

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Velociraptor Binary</Form.Label>
                    <Col sm="8">
                      {this.props.parameters.target_os === "Windows" &&
                       <ToolViewer name="VelociraptorWindows"/>}
                      {this.props.parameters.target_os === "Windows_x86" &&
                       <ToolViewer name="VelociraptorWindows_x86"/>}
                      {this.props.parameters.target_os === "Linux" &&
                       <ToolViewer name="VelociraptorLinux"/>}
                      {this.props.parameters.target_os === "MacOS" &&
                       <ToolViewer name="VelociraptorDarwin"/>}
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Temp directory</Form.Label>
                    <Col sm="8">
                      <Form.Control as="input"
                                    placeholder="Temp location"
                                    value={this.props.parameters.opt_tempdir}
                                    onChange={e => {
                                        this.props.parameters.opt_tempdir = e.target.value;
                                        this.props.setParameters(this.props.parameters);
                                    }}
                      />
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Compression Level</Form.Label>
                    <Col sm="8">
                      <ValidatedInteger setValue={value=>{
                          this.props.parameters.opt_level = value;
                          this.props.setParameters(this.props.parameters);
                      }}
                                        value={this.props.parameters.opt_level}
                                        placeholder="Compression Level (0 - 9)"
                      />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Output format</Form.Label>
                    <Col sm="8">
                      <Form.Control as="select"
                                    value={this.props.parameters.opt_format}
                                    onChange={(e) => {
                                        this.props.parameters.opt_format = e.target.value;
                                        this.props.setParameters(this.props.parameters);
                                    }}
                      >
                        <option value="jsonl">JSON</option>
                        <option value="csv">CSV and JSON</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>
                </Form>
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    step_name: "Offline Collector",
                }) }
              </Modal.Footer>
            </>
        );
    }
}


export default class OfflineCollectorWizard extends React.Component {
    static propTypes = {
        onResolve: PropTypes.func,
        onCancel: PropTypes.func,
    };

    state = {
        artifacts: [],
        parameters: {},
        collector_parameters: {
            target_os: "Windows",
            target: "ZIP",
            target_args: {
                // Common
                bucket: "",

                // For GCS Buckets
                GCSKey: "",

                // For S3 buckets.
                credentialsKey: "",
                credentialsSecret: "",
                region: "",
                endpoint: "",
                serverSideEncryption: "",
            },
            template: "",
            password: "",
            opt_level: 5,
            opt_format: "jsonl",
        },
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        this.setState({parameters: params});
    }

    // Build a request to the Server.Utils.CreateCollector artifact.
    prepareRequest = () => {
        let env = [];
        let request = {
            artifacts: ["Server.Utils.CreateCollector"],
            specs: [{artifact: "Server.Utils.CreateCollector", parameters: {env: env}}],
        };

        env.push({key: "OS",
                  value: this.state.collector_parameters.target_os});
        env.push({key: "artifacts", value: JSON.stringify(
            _.map(this.state.artifacts, (item) => item.name))});
        env.push({key: "parameters", value: JSON.stringify(this.state.parameters)});
        env.push({key: "target", value: this.state.collector_parameters.target});
        env.push({key: "target_args", value: JSON.stringify(
            this.state.collector_parameters.target_args)});
        env.push({key: "Password", value: this.state.collector_parameters.password});
        env.push({key: "template", value: this.state.collector_parameters.template});
        env.push({key: "opt_verbose", value: "Y"});
        env.push({key: "opt_banner", value: "Y"});
        env.push({key: "opt_prompt", value: "N"});
        env.push({key: "opt_admin", value: "Y"});
        env.push({key: "opt_tempdir", value: this.state.collector_parameters.opt_tempdir});
        env.push({key: "opt_level", value: this.state.collector_parameters.opt_level.toString()});
        env.push({key: "opt_format", value: this.state.collector_parameters.opt_format});

        return request;
    }

    launch = () => {
        this.props.onResolve(this.prepareRequest());
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

        let extra_tools = [
            "VelociraptorWindows",
            "VelociraptorLinux",
            "VelociraptorWindows_x86",
            "VelociraptorDarwin",
        ];

        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>
              <HotKeys keyMap={keymap} handlers={handlers}><ObserveKeys>
              <StepWizard ref={n=>this.step=n}>
                <NewCollectionSelectArtifacts
                  paginator={new OfflinePaginator(
                      "Select Artifacts",
                      "Create Offline collector: Select artifacts to collect",
                      (isFocused, step) => {
                          return isFocused;
                      })}
                  artifacts={this.state.artifacts}
                  artifactType="CLIENT"
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  parameters={this.state.parameters}
                  setParameters={this.setParameters}
                  artifacts={this.state.artifacts}
                  setArtifacts={this.setArtifacts}
                  paginator={new OfflinePaginator(
                      "Configure Parameters",
                      "Create Offline Collector: Configure artifact parameters")}
                />

                <OfflineCollectorParameters
                  setParameters={(x) => this.setState({collector_parameters: x})}
                  parameters={this.state.collector_parameters}
                  paginator={new OfflinePaginator(
                      "Configure Collection",
                      "Create Offline Collector: Configure collector")}
                />

                <NewCollectionRequest
                  paginator={new OfflinePaginator(
                      "Review",
                      "Create Offline Collector: Review request")}
                  request={this.prepareRequest()} />

                <NewCollectionLaunch
                  artifacts={this.state.artifacts}
                  tools={extra_tools}
                  paginator={new OfflinePaginator(
                      "Launch",
                      "Create Offline Collector: Create collector")}
                  launch={this.launch} />

              </StepWizard>
              </ObserveKeys></HotKeys>
            </Modal>
        );
    }
};
