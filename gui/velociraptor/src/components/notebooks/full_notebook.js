import React from 'react';
import NotebookRenderer from './notebook-renderer.js';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import axios from 'axios';
import api from '../core/api-service.js';
import Spinner from '../utils/spinner.js';
import { withRouter }  from "react-router-dom";

// Poll for new notebooks list.
const POLL_TIME = 5000;
const PAGE_SIZE = 100;

class FullScreenNotebook extends React.Component {
    state = {
        notebooks: [],
        selected_notebook: {},

        // Only show the spinner the first time the component is
        // mounted.
        loading: true,
    }

    componentDidMount = () => {
        this.source = axios.CancelToken.source();
        this.interval = setInterval(this.fetchNotebooks, POLL_TIME);
        this.fetchNotebooks();
    }

    componentWillUnmount() {
        this.source.cancel();
        clearInterval(this.interval);
    }

    fetchNotebooks = () => {
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = axios.CancelToken.source();

        api.get("v1/GetNotebooks", {
            count: PAGE_SIZE,
            offset: 0,
        }, this.source.token).then(response=>{
            if (response.cancel) return;

            let notebooks = response.data.items || [];
            let selected_notebook = {};

            // Check the router for a notebook id
            let notebook_id = this.props.match && this.props.match.params &&
                this.props.match.params.notebook_id;
            if (notebook_id) {
                for(var i = 0; i < notebooks.length; i++){
                    if (notebooks[i].notebook_id === notebook_id) {
                        selected_notebook = notebooks[i];
                        break;
                    }
                }
            }

            this.setState({notebooks: notebooks,
                           loading: false,
                           selected_notebook: selected_notebook});
        });
    }

    setSelectedNotebook = () => {
        this.props.history.push("/notebooks/" +
                                this.state.selected_notebook.notebook_id);
    }

    render() {
        return (
            <>
              <Spinner loading={this.state.loading} />
              <Navbar className="toolbar">
                <ButtonGroup>
                  <Button title="Exit Fullscreen"
                          onClick={this.setSelectedNotebook}
                          variant="default">
                    <FontAwesomeIcon icon="compress"/>
                  </Button>
                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins toolbar-margin selectable">
                <NotebookRenderer
                 fetchNotebooks={this.fetchNotebooks}
                 notebook={this.state.selected_notebook}
                />
              </div>
            </>
        );
    }
};

export default withRouter(FullScreenNotebook);
