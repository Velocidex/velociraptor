import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Modal from 'react-bootstrap/Modal';
import StepWizard from 'react-step-wizard';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';

import {
    NewCollectionSelectArtifacts,
    NewCollectionConfigParameters,
    NewCollectionResources,
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
                          <option value="Linux">Linux</option>
                          <option value="MacOS">Mac OS</option>
                        </Form.Control>
                      </Col>
                    </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Password</Form.Label>
                    <Col sm="8">
                      <Form.Control as="textarea" rows={3}
                                    placeholder="Password"
                                    value={this.props.parameters.password}
                                    onChange={e => {
                                        this.props.parameters.password = e.target.value;
                                        this.props.setParameters(this.props.parameters);
                                    }} />
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
                      </Form.Control>
                    </Col>
                  </Form.Group>

                  { this.props.parameters.target == "GCS" && <>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">GCS Bucket</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Bucket name"
                                      value={this.props.parameters.target_args.gcs_bucket}
                                      onChange={e => {
                                          this.props.parameters.target_args.gcs_bucket = e.target.value;
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
                                      value={this.props.parameters.target_args.gcs_key_blob}
                                      onChange={e => {
                                          this.props.parameters.target_args.gcs_key_blob = e.target.value;
                                          this.props.setParameters(this.props.parameters);
                                      }}
                        />
                      </Col>
                    </Form.Group> </>

                  }
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
                gcs_bucket: "",
                gcs_key_blob: "",
            },
            template: "Reporting.Default",
            password: "",
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
            parameters: {env: env}};

        env.push({key: "OS",
                  value: this.state.collector_parameters.target_os});
        env.push({key: "artifacts", value: JSON.stringify(
            _.map(this.state.artifacts, (item) => item.name))});
        env.push({key: "parameters", value: JSON.stringify(this.state.parameters)});
        env.push({key: "target", value: this.state.collector_parameters.target});
        env.push({key: "target_args", value: JSON.stringify(
            this.state.collector_parameters.target_args)});
        env.push({key: "opt_verbose", value: "Y"});
        env.push({key: "opt_banner", value: "Y"});
        env.push({key: "opt_prompt", value: "Y"});
        env.push({key: "opt_admin", value: "Y"});

        return request;
    }

    launch = () => {
        this.props.onResolve(this.prepareRequest());
    }

    render() {
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>

              <StepWizard>
                <NewCollectionSelectArtifacts
                  paginator={new OfflinePaginator(
                      "Select Artifacts",
                      "Create Offline collector: Select artifacts to collect",
                      (isFocused, step) => {
                          return isFocused;
                      })}
                  artifacts={this.state.artifacts}
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
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
                  launch={this.launch} />

              </StepWizard>
            </Modal>
        );
    }
};
