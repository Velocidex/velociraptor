import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Modal from 'react-bootstrap/Modal';
import StepWizard from 'react-step-wizard';
import Form from 'react-bootstrap/Form';
import Row from 'react-bootstrap/Row';
import Col from 'react-bootstrap/Col';
import LabelForm from '../utils/labels.js';

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
        setParameters: PropTypes.func.isRequired,
    }

    state = {
        target_os: "Windows",
        target: "ZIP",
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
                                      value={this.state.target_os}
                                      onChange={(e) => this.setState({
                                          target_os: e.currentTarget.value})}
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
                                    onChange={e => this.setState({password: e.target.value})} />
                    </Col>
                  </Form.Group>

                  <Form.Group as={Row}>
                    <Form.Label column sm="3">Collection Type</Form.Label>
                    <Col sm="8">
                      <Form.Control as="select"
                                    value={this.state.target}
                                    onChange={(e) => this.setState({
                                        target: e.currentTarget.value})}
                      >
                        <option value="ZIP">Zip Archive</option>
                        <option value="GCS">Google Cloud Bucket</option>
                        <option value="S3">AWS Bucket</option>
                      </Form.Control>
                    </Col>
                  </Form.Group>

                  { this.state.target == "GCS" && <>
                    <Form.Group as={Row}>
                      <Form.Label column sm="3">GCS Bucket</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="Bucket name"
                                      onChange={e => this.setState({bucket: e.target.value})} />
                      </Col>
                    </Form.Group>

                    <Form.Group as={Row}>
                      <Form.Label column sm="3">GCS Key Blob</Form.Label>
                      <Col sm="8">
                        <Form.Control as="textarea" rows={3}
                                      placeholder="GCS Blob"
                                      onChange={e => this.setState({gcs_blob: e.target.value})} />
                      </Col>
                    </Form.Group> </>

                  }
                </Form>
              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                    step_name: "Offline Collector",
                    onBlur: () => this.props.setParameters(this.state),
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
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        this.setState({parameters: params});
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

        return request;
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
                  paginator={new OfflinePaginator(
                      "Configure Parameters",
                      "Create Offline Collector: Configure artifact parameters")}
                />

                <OfflineCollectorParameters
                  setParameters={(x) => this.setState({collector_parameters: x})}
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
