import React from 'react';
import PropTypes from 'prop-types';

import _ from 'lodash';
import api from '../core/api-service.js';

import Modal from 'react-bootstrap/Modal';

import StepWizard from 'react-step-wizard';

import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';

import DateTimePicker from 'react-datetime-picker';

import LabelForm from '../utils/labels.js';

import {
    NewCollectionSelectArtifacts,
    NewCollectionConfigParameters,
    NewCollectionResources,
    NewCollectionRequest,
    NewCollectionLaunch,
    PaginationBuilder
} from '../flows/new-collection.js';


// The hunt wizard is built upon the new collection wizard with some extra steps.
class HuntPaginator extends PaginationBuilder {
    PaginationSteps = ["Configure Hunt",
                       "Select Artifacts", "Configure Parameters",
                       "Specify Resorces", "Review", "Launch"];
}

class NewHuntConfigureHunt extends React.Component {
    static propTypes = {
        setHuntParameters: PropTypes.func,
        paginator: PropTypes.object,
    }

    constructor(props) {
        super(props);
        this.state = {
            // By default a week in the future.
            expires: new Date(),

            include_condition: "",
            include_labels: [],
            include_os: "",
            exclude_condition: "",
            excluded_labels: [],
        };

        let now = new Date();
        this.state.expires.setTime(now.getTime() + 7 * 24 * 60 * 60 * 1000);
    };

    render() {
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>New Hunt - Configure Hunt</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Form>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Description</Form.Label>
                    <Col sm="8">
                      <Form.Control as="textarea" rows={3}
                                    placeholder="Hunt description"
                                    onChange={e => this.setState({description: e.target.value})} />
                    </Col>
                  </Form.Group>
                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Expiry</Form.Label>
                    <Col sm="8">
                      <DateTimePicker value={this.state.expires}
                        onChange={(value) => this.setState({expires: value})}
                      />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Include Condition</Form.Label>
                    <Col sm="8">
                        <Form.Control as="select"
                          value={this.state.include_condition}
                          onChange={(e) => this.setState({
                              include_condition: e.currentTarget.value})}
                          >
                          <option label="Run everywhere" value="">Run everywhere</option>
                          <option label="Match by label" value="labels">Match by label</option>
                          <option label="Operating System" value="os">Operating System</option>
                        </Form.Control>
                    </Col>
                  </Form.Group>

                  { this.state.include_condition == "os" &&
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Operating System Included</Form.Label>
                      <Col sm="8">
                        <Form.Control as="select"
                                      value={this.state.include_os}
                                      onChange={(e) => this.setState({
                                          include_os: e.currentTarget.value})}
                        >
                          <option label="Windows" value="WINDOWS">Windows</option>
                          <option label="Linux" value="LINUX">LINUX</option>
                          <option label="MacOS" value="OSX">OSX</option>
                        </Form.Control>
                      </Col>
                    </Form.Group>
                  }

                  { this.state.include_condition == "labels" &&
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Include Labels</Form.Label>
                      <Col sm="8">
                        <LabelForm
                          value={this.state.include_labels}
                          onChange={(value) => this.setState({include_labels: value})}
                        />
                      </Col>
                    </Form.Group>
                  }

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Exclude Condition</Form.Label>
                    <Col sm="8">
                        <Form.Control as="select"
                          value={this.state.exclude_condition}
                          onChange={(e) => this.setState({
                              exclude_condition: e.currentTarget.value})}
                          >
                          <option label="Run everywhere" value="">Run everywhere</option>
                          <option label="Match by label" value="labels">Match by label</option>
                        </Form.Control>
                    </Col>
                  </Form.Group>

                  { this.state.exclude_condition == "labels" &&
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">Exclude Labels</Form.Label>
                      <Col sm="8">
                        <LabelForm
                          value={this.state.excluded_labels}
                          onChange={(value) => this.setState({excluded_labels: value})}
                        />
                      </Col>
                    </Form.Group>
                  }

                </Form>
                { JSON.stringify(this.state)}
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    step_name: "Configure Hunt",
                    onBlur: () => this.props.setHuntParameters(this.state),
                }) }
              </Modal.Footer>
            </>
        );
    }
}


export default class NewHuntWizard extends React.Component {

    static propTypes = {
        baseFlow: PropTypes.object,
        onResolve: PropTypes.func,
        onCancel: PropTypes.func,
    }

    state = {
        // A list of artifact descriptors we have selected so far.
        artifacts: [],

        // A key/value mapping of edited parameters by the user.
        parameters: {env: []},

        resources: {},

        hunt_parameters: {},
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        let new_parameters = {env: []};
        for (var k in params) {
            if (params.hasOwnProperty(k)) {
                new_parameters.env.push({key: k, value: params[k]});
            }
        }
        this.setState({parameters: new_parameters});
    }

    setResources = (resources) => {
        this.setState({resources: resources});
    }

    // Let our caller know the artifact request we created.
    launch = () => {
        this.props.onResolve(this.prepareRequest());
    }

    prepareRequest = () => {
        let artifacts = [];
        _.each(this.state.artifacts, (item) => {
            artifacts.push(item.name);
        });

        let request = {
            artifacts: artifacts,
            parameters: this.state.parameters,
        };

        if (this.state.resources.ops_per_second) {
            request.ops_per_second = this.state.resources.ops_per_second;
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
            result.expires = hunt_parameters.expires.getTime() * 1000000;
        }

        if (hunt_parameters.include_condition === "labels") {
            result.condition.labels = {"label": hunt_parameters.include_labels};
        }
        if (hunt_parameters.include_condition === "os") {
            result.condition.os = {"os": hunt_parameters.include_os};
        }

        if (hunt_parameters.exclude_condition === "labels") {
            result.condition.excluded_labels = hunt_parameters.excluded_labels;
        }

        if (hunt_parameters.description) {
            result.description = hunt_parameters.description;
        }

        return result;
    }

    render() {
        console.log(this.prepareRequest());
        return (
            <Modal show={true}
                   dialogClassName="modal-90w"
                   enforceFocus={false}
                   scrollable={true}
                   onHide={this.props.onCancel}>

              <StepWizard>
                <NewHuntConfigureHunt
                  paginator={new HuntPaginator("Configure Hunt",
                                               "Create Hunt: Configure hunt")}
                  setHuntParameters={x => this.setState({hunt_parameters: x})}
                />

                <NewCollectionSelectArtifacts
                  paginator={new HuntPaginator("Select Artifacts",
                                               "Create Hunt: Select artifacts to collect")}
                  artifacts={this.state.artifacts}
                  setArtifacts={this.setArtifacts}/>

                <NewCollectionConfigParameters
                  setParameters={this.setParameters}
                  artifacts={this.state.artifacts}
                  paginator={new HuntPaginator("Configure Parameters",
                                               "Create Hunt: Configure artifact parameters")}
                />

                <NewCollectionResources
                  paginator={new HuntPaginator("Specify Resorces",
                                               "Create Hunt: Specify resource limits")}

                  setResources={this.setResources} />

                <NewCollectionRequest
                  paginator={new HuntPaginator("Review",
                                              "Create Hunt: Review request")}
                  request={this.prepareRequest()} />

                <NewCollectionLaunch
                  launch={this.launch} />

              </StepWizard>
            </Modal>
        );
    }
}
