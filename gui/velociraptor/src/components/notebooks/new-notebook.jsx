import _ from 'lodash';

import React from 'react';
import PropTypes from 'prop-types';
import {CancelToken} from 'axios';
import Modal from 'react-bootstrap/Modal';
import Form from 'react-bootstrap/Form';
import Col from 'react-bootstrap/Col';
import Row from 'react-bootstrap/Row';
import Button from 'react-bootstrap/Button';
import StepWizard from 'react-step-wizard';

import {
    NewCollectionSelectArtifacts,
    NewCollectionRequest,
    PaginationBuilder,
} from '../flows/new-collection.jsx';

import T from '../i8n/i8n.jsx';

import UserForm from '../utils/users.jsx';

import api from '../core/api-service.jsx';


class NotebookPaginationBuilder extends PaginationBuilder {
    PaginationSteps = [T("Configure Parameters"), T("Select Template"),
                       T("Review"), T("Launch")];
}

class NewNotebookParameters extends React.Component {
    static propTypes = {
        parameters: PropTypes.object,
        setParameters: PropTypes.func.isRequired,
        paginator: PropTypes.object,
    };

    setValue = (x) => {
        let params = this.props.parameters || {};
        _.forOwn(x, (v,k)=>{
            params[k]=v;
        });
        this.props.setParameters(params);
    }

    render() {
        let params = this.props.parameters || {};
        return (
            <>
              <Modal.Header closeButton>
                <Modal.Title>{ T(this.props.paginator.title) }</Modal.Title>
              </Modal.Header>

              <Modal.Body className="new-collection-parameter-page selectable">
                <Form.Group as={Row}>
                  <Form.Label column sm="3">Name</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  value={params.name}
                                  onChange={(e) => this.setValue(
                                      {name: e.currentTarget.value})} />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Description")}</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  value={params.description}
                                  onChange={(e) => this.setValue(
                                      {description: e.currentTarget.value})} />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Public")}</Form.Label>
                  <Col sm="8">
                    <Form.Check
                      type="checkbox"
                      label="Share with all users"
                      value={params.public || false}
                      onChange={(e) => this.setValue(
                          {public: e.currentTarget.checked})}/>
                  </Col>
                </Form.Group>

                { !params.public &&
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Collaborators")}</Form.Label>
                  <Col sm="8">
                    <UserForm
                      value={params.collaborators}
                      onChange={(value) => this.setValue({collaborators: value})}/>
                  </Col>
                </Form.Group>}

              </Modal.Body>
              <Modal.Footer>
                { this.props.paginator.makePaginator({
                    props: this.props,
                }) }
              </Modal.Footer>
            </>
        );
    };
}

class NewNotebookLaunch extends React.Component {
    static propTypes = {
        artifacts: PropTypes.array,
        launch: PropTypes.func,
        isActive: PropTypes.bool,
        paginator: PropTypes.object,
    }

    componentDidMount() {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.isActive && !prevProps.isActive) {
            this.props.launch();
        }
    }

    state = {
        current_error: "",
    }

    render() {
        return <>
                 <Modal.Header closeButton>
                   <Modal.Title>{ T(this.props.paginator.title) }</Modal.Title>
                 </Modal.Header>
                 <Modal.Body>
                 </Modal.Body>
                 <Modal.Footer>
                   { this.props.paginator.makePaginator({
                       props: this.props,
                       isFocused: ()=>this.props.launch(),
                   }) }
                 </Modal.Footer>
               </>;
    }
}

export class NewNotebook extends React.Component {
    static propTypes = {
        notebook: PropTypes.object,
        closeDialog: PropTypes.func.isRequired,
        updateNotebooks: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        if(!_.isEmpty(this.props.notebook)) {
            this.setState({parameters: {
                name: this.props.notebook.name,
                notebook_id: this.props.notebook.notebook_id,
                public: this.props.notebook.public,
                description: this.props.notebook.description,
                modified_time: this.props.notebook.modified_time,
                cell_metadata: this.props.notebook.cell_metadata,
                collaborators: this.props.notebook.collaborators || [],
            }});
        }
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    state = {
        // Filled by step 1
        artifacts: [],

        // Filled by step 2
        parameters: {
            name: "New Notebook",
        },
    }

    setArtifacts = (artifacts) => {
        this.setState({artifacts: artifacts});
    }

    setParameters = (params) => {
        this.setState({parameters: params});
    }

    launch = () =>{
        let api_url = "v1/NewNotebook";
        api.post(api_url, this.prepareRequest(), this.source.token).
            then(this.props.updateNotebooks);
    }

    prepareRequest = () => {
        let p = this.state.parameters || {};
        return {
            name: p.name,
            description: p.description,
            collaborators: p.collaborators,
            public: p.public,
            artifacts: _.map(this.state.artifacts || [], x=>x.name),
        };
    }

    render() {
        return (
            <Modal show={true}
                   className="full-height"
                   dialogClassName="modal-90w"
                   scrollable={true}
                   onHide={this.props.closeDialog}>
              <StepWizard ref={n=>this.step=n}>
                <NewNotebookParameters
                  parameters={this.state.parameters}
                  setParameters={this.setParameters}
                  paginator={new NotebookPaginationBuilder(
                      T("Configure Parameters"),
                      T("New Notebook: Configure Parameters"))}
                />
                <NewCollectionSelectArtifacts
                  artifacts={this.state.artifacts}
                  artifactType={"NOTEBOOK"}
                  paginator={new NotebookPaginationBuilder(
                      T("Select Template"),
                      T("New Notebook: Select Notebook template Artifact"))}
                  setArtifacts={this.setArtifacts}
                  setParameters={p=>1}
                />

                <NewCollectionRequest
                  paginator={new NotebookPaginationBuilder(
                      T("Review"),
                      T("New Collection: Review request"))}
                  request={this.prepareRequest()} />

                <NewNotebookLaunch
                  artifacts={this.state.artifacts}
                  paginator={new NotebookPaginationBuilder(
                      T("Launch"),
                      T("New Notebook: Launch collection"))}
                  launch={this.launch} />
              </StepWizard>
            </Modal>
        );
    }
}




export class EditNotebook extends React.Component {
    static propTypes = {
        notebook: PropTypes.object,
        closeDialog: PropTypes.func.isRequired,
        updateNotebooks: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        if(!_.isEmpty(this.props.notebook)) {
            this.setState({
                name: this.props.notebook.name,
                notebook_id: this.props.notebook.notebook_id,
                public: this.props.notebook.public,
                description: this.props.notebook.description,
                modified_time: this.props.notebook.modified_time,
                cell_metadata: this.props.notebook.cell_metadata,
                collaborators: this.props.notebook.collaborators || [],
            });
        }
    }

    componentWillUnmount() {
        this.source.cancel("unmounted");
    }

    newNotebook = () => {
        let api_url = "v1/UpdateNotebook";
        api.post(api_url, {
            name: this.state.name,
            description: this.state.description,
            public: this.state.public,
            collaborators: this.state.collaborators,
            modified_time: this.state.modified_time,
            notebook_id: this.state.notebook_id,
            cell_metadata: this.state.cell_metadata,
        }, this.source.token).then(this.props.updateNotebooks);
    }

    state = {
        name: "",
        description: "",
        collaborators: [],
        users: [],
        public: false,
        notebook_id: undefined,
        modified_time: undefined,
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>
                  {_.isEmpty(this.props.notebook) ?
                   T("Create a new Notebook") :
                   T("Edit notebook ") + this.props.notebook.notebook_id}
                </Modal.Title>
              </Modal.Header>

              <Modal.Body>
                <Form.Group as={Row}>
                  <Form.Label column sm="3">Name</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  value={this.state.name}
                                  onChange={(e) => this.setState(
                                      {name: e.currentTarget.value})} />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Description")}</Form.Label>
                  <Col sm="8">
                    <Form.Control as="textarea"
                                  rows={1}
                                  value={this.state.description}
                                  onChange={(e) => this.setState(
                                      {description: e.currentTarget.value})} />
                  </Col>
                </Form.Group>

                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Public")}</Form.Label>
                  <Col sm="8">
                    <Form.Check
                      type="checkbox"
                      label="Share with all users"
                      checked={this.state.public}
                      onChange={(e) => this.setState(
                          {public: e.currentTarget.checked})}/>
                  </Col>
                </Form.Group>

                { !this.state.public &&
                <Form.Group as={Row}>
                  <Form.Label column sm="3">{T("Collaborators")}</Form.Label>
                  <Col sm="8">
                    <UserForm
                      value={this.state.collaborators}
                      onChange={(value) => this.setState({collaborators: value})}/>
                  </Col>
                </Form.Group>}

              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Cancel")}
                </Button>
                <Button variant="primary"
                        onClick={this.newNotebook}>
                  {T("Submit")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
