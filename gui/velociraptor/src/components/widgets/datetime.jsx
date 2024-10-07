import _ from 'lodash';
import './datetime.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import UserConfig from '../core/user.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import InputGroup from 'react-bootstrap/InputGroup';
import ToolTip from '../widgets/tooltip.jsx';
import Row from 'react-bootstrap/Row';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Dropdown from 'react-bootstrap/Dropdown';
import moment from 'moment';
import 'moment-timezone';

import Form from 'react-bootstrap/Form';

import VeloTimestamp, {
    ToStandardTime,
    FormatRFC3339,
} from '../utils/time.jsx';

class Calendar extends Component {
    static contextType = UserConfig;

    static propTypes = {
        // A string in RFC3339 format.
        value: PropTypes.string,

        // Update the timestamp with the RFC3339 string.
        onChange: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        let ts = this.parseDate(this.props.value);
        if(ts.isValid()) {
            this.setState({focus: ts});
        }
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (_.isUndefined(this.state.focus) ||
            this.props.value !== prevProps.value) {
            let ts = this.parseDate(this.props.value);
            if(ts.isValid()) {
                this.setState({focus: ts});
            }
        }
    }

    parseDate = x=>{
        let timezone = this.context.traits.timezone || "UTC";
        let ts = ToStandardTime(x);
        return moment(ts, timezone).tz(timezone);
    }

    now = ()=>{
        let timezone = this.context.traits.timezone || "UTC";
        return moment().tz(timezone);
    }

    setTime = t=>{
        let timezone = this.context.traits.timezone || "UTC";
        let str = FormatRFC3339(t, timezone);
        this.setState({edited_value: str, invalid: false});
        this.props.onChange(str);
    }

    getMonthName = x=>{
        switch (x.month() % 12) {
        case 0: return T("January");
        case 1: return T("February");
        case 2: return T("March");
        case 3: return T("April");
        case 4: return T("May");
        case 5: return T("June");
        case 6: return T("July");
        case 7: return T("August");
        case 8: return T("September");
        case 9: return T("October");
        case 10: return T("November");
        case 11: return T("December");
        default: return "";
        };
    }

    renderDay = (ts, focus, base)=>{
        let classname = "calendar-day";
        let day_of_month = ts.date();
        if (day_of_month === base.date() &&
            ts.month() == base.month() &&
            ts.year() == base.year()) {
            classname = "calendar-selected-date";
        }

        if (ts.month() !== focus.month()) {
            classname = "calendar-other-day";
        }
        return <div
                 onClick={()=>{
                     let x = moment(ts).
                         millisecond(0).hours(0).minutes(0).seconds(0);
                     this.setTime(x);
                 }}
                 className={classname}>
                 {day_of_month.toString()}
               </div>;
    }

    renderWeek = (ts, focus, base)=>{
        let res = [];
        for(let j=0;j<7;j++) {
            let day_ts = moment(ts).add(j, "days");
            res.push(<td key={j}>
                       {this.renderDay(day_ts, focus, base)}
                     </td>);
        }
        return res;
    }

    renderMonth = (ts, focus, base)=>{
        let res = [];
        for(let j=0;j<=5;j++) {
            let week_start = moment(ts).add(j, "weeks");
            if (week_start.year() == focus.year() &&
                week_start.month() > focus.month()) {
                break;
            }
            res.push(<tr key={j}
                         className="calendar-week">
                       {this.renderWeek(week_start, focus, base)}
                     </tr>);
        }
        return <table className="calendar-month">
                 <tbody>
                   {res}
                 </tbody>
               </table>;
    }

    // Figure out where to start rendering the calendar. We start at the
    // first day of the first week of the month. This means we need
    // to go back a bit into the previous month to get thee first full
    // week start.
    getFirstDayToRender = ts=>{
        let first_day_of_month = moment(ts).date(1);
        let day_of_week = first_day_of_month.day();
        let first_day_to_render = first_day_of_month.subtract(day_of_week, "days");

        return first_day_to_render;
    }

    state = {
        // The date the calendar is currently focused on. As the user
        // navigates the calendar this focus will change.
        focus: undefined,
    }

    render() {
        let ts = this.parseDate(this.props.value);
        if (!ts.isValid()) {
            // use current time.
            ts = this.now();
        }

        // Where the calendar will be focused on.
        let focus = this.state.focus;
        if (_.isUndefined(focus)) {
            focus = moment(ts);
        }
        let start = this.getFirstDayToRender(focus);
        return <div className="calendar-selector">
                 <Row>
                   <ButtonGroup>
                     <Button onClick={e=>{
                         this.setState({focus: moment(focus).subtract(1, "months")});

                         e.stopPropagation();
                         e.preventDefault();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="backward"/>
                     </Button>
                     <Button>
                       {this.getMonthName(focus)}
                     </Button>
                     <Button onClick={e=>{
                         this.setState({focus: moment(focus).add(1, "months")});

                         e.preventDefault();
                         e.stopPropagation();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="forward"/>
                     </Button>
                   </ButtonGroup>
                 </Row>
                 <Row>
                   {this.renderMonth(start, focus, ts)}
                 </Row>
                 <Row>
                   <ButtonGroup>
                     <Button onClick={e=>{
                         this.setState({focus: moment(focus).subtract(1, "years")});

                         e.stopPropagation();
                         e.preventDefault();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="backward"/>
                     </Button>
                     <Button>
                       {T("Year") + " " + focus.year().toString()}
                     </Button>
                     <Button onClick={e=>{
                         this.setState({focus: moment(focus).add(1, "years")});

                         e.stopPropagation();
                         e.preventDefault();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="forward"/>
                     </Button>
                   </ButtonGroup>
                 </Row>
               </div>;
    }
}


export default class DateTimePicker extends Component {
    static contextType = UserConfig;

    static propTypes = {
        // A string in RFC3339 format.
        value: PropTypes.string,

        // Update the timestamp with the RFC3339 string.
        onChange: PropTypes.func.isRequired,
    }

    state = {
        // Current edited time - it will be parsed from RFC3339 format
        // when the user clicks the save button.
        edited_value: "",
        invalid: false,
    }

    componentDidMount() {
        this.setTime(this.props.value);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.value !== prevProps.value) {
            this.setTime(this.props.value);
        }
    }

    setTime = t=>{
        // Make sure the time is valid before setting it but we still
        // set the RFC3339 string.
        if(_.isString(t)) {
            let ts = ToStandardTime(t);
            if (_.isDate(ts)) {
                this.setState({edited_value: t, invalid: false});
                this.props.onChange(t);
            }
        }
    }

    render() {
        let formatted_ts = this.props.value;
        let timezone = this.context.traits.timezone || "UTC";

        let presets = [
            [()=>T("A week from now"), ()=>moment().add(1, "week")],
            [()=>T("Now"), ()=>moment()],
            [()=>T("A day ago"), ()=>moment().subtract(1, "day")],
            [()=>T("A week ago"), ()=>moment().subtract(1, "week")],
            [()=>T("A month ago"), ()=>moment().subtract(1, "month")],
        ];

        return (
            <>
              <Dropdown as={InputGroup} className="datetime-selector">
                <Button
                  onClick={()=>{
                       this.setState({showEdit: !this.state.showEdit});
                  }}
                  variant="default-outline">
                  { formatted_ts ? <VeloTimestamp iso={formatted_ts}/> : T("Select Time")}
                </Button>

                {this.state.showEdit &&
                 <>
                   <Button variant="default"
                           type="submit"
                           disabled={this.state.invalid}
                           onClick={()=>{
                               this.setTime(this.state.edited_value);
                               this.setState({showEdit: false});
                           }}>
                     <FontAwesomeIcon icon="save"/>
                   </Button>
                   <ToolTip tooltip={T("Specify date and time in RFC3339 format")}>
                     <Form.Control as="input" rows={1}
                                   spellCheck="false"
                                   className={ this.state.invalid && 'invalid' }
                                   value={this.state.edited_value}

                                   onChange={e=>{
                                       // Check if the time is invalid
                                       // - as the user edits the time
                                       // it may become invalid at any
                                       // time.
                                       let ts = ToStandardTime(e.target.value);
                                       this.setState({
                                           edited_value: e.target.value,
                                           invalid: !_.isDate(ts),
                                       });
                                   }}
                     />
                   </ToolTip>
                 </>
                }

                <Dropdown.Toggle split variant="success">
                  <span className="icon-small">
                    <FontAwesomeIcon icon="sliders"/>
                  </span>
                </Dropdown.Toggle>
                <Dropdown.Menu>
                  <Dropdown.Item className="calendar-selector">
                    <Calendar value={this.props.value}
                              onChange={this.props.onChange}/>
                  </Dropdown.Item>

                  { _.map(presets, (x, idx)=>{
                      let new_ts = x[1]();
                      return <Dropdown.Item
                               key={idx}
                               onClick={e=>this.setTime(
                                   FormatRFC3339(new_ts, timezone))}
                             >
                               {x[0]()}
                             </Dropdown.Item>;
                  })}
                  <Dropdown.Divider />

                </Dropdown.Menu>
              </Dropdown>
              </>
        );
    }
}
