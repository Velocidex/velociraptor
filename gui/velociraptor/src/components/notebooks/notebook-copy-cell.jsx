import _ from 'lodash';

import PropTypes from 'prop-types';
import React, { Component } from 'react';

import Modal from 'react-bootstrap/Modal';
import T from '../i8n/i8n.jsx';
import Button from 'react-bootstrap/Button';
import Spinner from '../utils/spinner.jsx';
import NotebooksList from './notebooks-list.jsx';

import api from '../core/api-service.jsx';
import {CancelToken} from 'axios';

const PAGE_SIZE = 100;


export default class CopyCellToNotebookDialog extends Component {
    static propTypes = {
        closeDialog: PropTypes.func.isRequired,
        cell: PropTypes.object,
        notebook_metadata: PropTypes.object,
    }

    state = {
        notebooks: [],
        notebook_id: null,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
        this.fetchNotebooks();
    }

    copyCell = ()=>{
        if(!this.props.notebook_metadata || !this.state.selected_notebook) {
            return;
        }

        let env = this.props.cell.env || [];
        env = _.uniqBy(env.concat(this.props.notebook_metadata.env || []),
                     x=>x.key);

        let new_cell = {
            notebook_id: this.state.selected_notebook.notebook_id,
            type: this.props.cell.type,
            input: this.props.cell.input,
            env: env,
        };

        // Add env variables as explicit VQL to make it clearer this
        // cell came from an external notebook.
        if (this.props.cell.type === "vql") {
            _.each(env, x=>{
                switch(x.key) {
                    case "ArtifactName":
                    case "FlowId":
                    case "ClientId":
                    case "HuntId":
                    new_cell.input = `LET ${x.key} <= '''${x.value}''' \n` + new_cell.input;
                }
            });
        }

        this.setState({loading: true});
        api.post('v1/NewNotebookCell',
                 new_cell,
                 this.source.token).then((response) => {
                     if (response.cancel) return;
                     this.setState({loading: false});
                     this.props.closeDialog();
                 });
    }

    fetchNotebooks = () => {
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        api.get("v1/GetNotebooks", {
            count: PAGE_SIZE,
            offset: 0,
        }, this.source.token).then(response=>{
            if (response.cancel) return;

            let notebooks = response.data.items || [];
            this.setState({notebooks: notebooks,
                           loading: false});
        });
    }

    render() {
        return (
            <Modal show={true}
                   size="lg"
                   className="full-height"
                   dialogClassName="modal-90w"
                   onHide={this.props.closeDialog} >
              <Modal.Header closeButton>
                <Modal.Title>{T("Copy Cell To Global Notebook")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <h3>{T("Select a notebook to append this cell to ...")}</h3>
              <Spinner loading={this.state.loading} />
                <NotebooksList
                  fetchNotebooks={this.fetchNotebooks}
                  selected_notebook={this.state.selected_notebook}
                  setSelectedNotebook={x=>this.setState({selected_notebook:x})}
                  notebooks={this.state.notebooks}
                  hideToolbar={true}
                />
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary"
                        onClick={this.props.closeDialog}>
                  {T("Cancel")}
                </Button>
                <Button variant="primary"
                        disabled={!this.state.selected_notebook}
                        onClick={() => this.copyCell(this.state.notebook_id)}>
                  {T("Submit")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
