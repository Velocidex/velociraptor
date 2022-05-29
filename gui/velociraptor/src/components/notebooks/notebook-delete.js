import React from 'react';
import PropTypes from 'prop-types';
import { runArtifact } from "../flows/utils.js";
import Modal from 'react-bootstrap/Modal';
import Spinner from '../utils/spinner.js';
import Button from 'react-bootstrap/Button';
import axios from 'axios';

import T from '../i8n/i8n.js';

export default class DeleteNotebookDialog extends React.PureComponent {
    static propTypes = {
        notebook_id: PropTypes.string,
        onClose: PropTypes.func.isRequired,
    }

    state = {
        loading: false,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    startDeleteNotebook = () => {
        if (this.props.notebook_id) {
            this.setState({loading: true});
            runArtifact("server", // This collection happens on the server.
                        "Server.Utils.DeleteNotebook",
                        {NotebookId: this.props.notebook_id,
                         ReallyDoIt: "Y"}, ()=>{
                             this.props.onClose();
                             this.setState({loading: false});
                         }, this.source.token);
        }
    }

    render() {
        return (
            <Modal show={true} onHide={this.props.onClose}>
              <Modal.Header closeButton>
                <Modal.Title>{T("Permanently delete Notebook")}</Modal.Title>
              </Modal.Header>
              <Modal.Body>
                <Spinner loading={this.state.loading} />
                {T("You are about to permanently delete the notebook for this hunt")}
              </Modal.Body>
              <Modal.Footer>
                <Button variant="secondary" onClick={this.props.onClose}>
                  {T("Close")}
                </Button>
                <Button variant="primary" onClick={this.startDeleteNotebook}>
                  {T("Yes do it!")}
                </Button>
              </Modal.Footer>
            </Modal>
        );
    }
}
