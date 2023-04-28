import React from 'react';
import PropTypes from 'prop-types';

import NotebookRenderer from './notebook-renderer.jsx';
import Navbar from 'react-bootstrap/Navbar';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import {CancelToken} from 'axios';
import api from '../core/api-service.jsx';
import Spinner from '../utils/spinner.jsx';
import { withRouter }  from "react-router-dom";
import T from '../i8n/i8n.jsx';

// Poll for new notebooks list.
const POLL_TIME = 5000;
const PAGE_SIZE = 100;

class FullScreenNotebook extends React.Component {
    static propTypes = {
        // React router props.
        match: PropTypes.object,
        history: PropTypes.object,
    }

    state = {
        notebooks: [],
        selected_notebook: {},

        // Only show the spinner the first time the component is
        // mounted.
        loading: true,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
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
        this.source = CancelToken.source();

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
                <ButtonGroup className="float-right floating-button">
                  <Button data-tooltip={T("Exit Fullscreen")}
                          data-position="left"
                          className="btn-tooltip"
                          onClick={this.setSelectedNotebook}
                          variant="default">
                    <FontAwesomeIcon icon="compress"/>
                  </Button>
                </ButtonGroup>
              </Navbar>
              <div className="fill-parent no-margins selectable">
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
